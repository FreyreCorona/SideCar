//go:build windows

package metrics

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// CollectCPUMetrics reads CPU load and temperature on Windows.
// Load is obtained from typeperf; temperature via WMIC (may return 0 on some systems).
func CollectCPUMetrics() CPUMetrics {
	return CPUMetrics{
		UsagePercent: getWindowsCPUUsage(),
		Temperature:  getWindowsCPUTemperature(),
	}
}

// CollectMemoryMetrics reads total and used RAM on Windows via WMIC.
func CollectMemoryMetrics() MemoryMetrics {
	used, total := getWindowsMemory()
	return MemoryMetrics{UsedMB: used, TotalMB: total}
}

// CollectUptimeMetrics reads system uptime on Windows.
func CollectUptimeMetrics() UptimeMetrics {
	return UptimeMetrics{Seconds: getWindowsUptime()}
}

// CollectNetworkMetrics reads network counters on Windows via netstat.
func CollectNetworkMetrics() NetworkMetrics {
	return getWindowsNetworkMetrics()
}

// CollectBatteryMetrics reads battery status on Windows via WMIC.
func CollectBatteryMetrics() BatteryMetrics {
	return getWindowsBattery()
}

// ─── helpers ───────────────────────────────────────────────────────────────

func getWindowsCPUUsage() float64 {
	// typeperf samples the Performance Counter once and exits.
	// Output format: "(PDH-CSV 4.0),(\\Machine\Processor(_Total)\% Processor Time)"
	//                "timestamp","value"
	out, err := runPS(`(Get-Counter '\Processor(_Total)\% Processor Time').CounterSamples[0].CookedValue`)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
	return v
}

func getWindowsCPUTemperature() float64 {
	// MSAcpi_ThermalZoneTemperature is only available on some hardware.
	out, err := runPS(`
		$t = Get-WmiObject MSAcpi_ThermalZoneTemperature -Namespace "root/wmi" 2>$null |
		     Select-Object -First 1 -ExpandProperty CurrentTemperature
		if ($t) { ($t - 2732) / 10 } else { 0 }
	`)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
	return v
}

func getWindowsMemory() (usedMB, totalMB int) {
	out, err := runPS(`
		$os = Get-CimInstance Win32_OperatingSystem
		"$($os.TotalVisibleMemorySize) $($os.FreePhysicalMemory)"
	`)
	if err != nil {
		return
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) < 2 {
		return
	}
	total, _ := strconv.ParseInt(parts[0], 10, 64)
	free, _ := strconv.ParseInt(parts[1], 10, 64)
	totalMB = int(total / 1024)
	usedMB = int((total - free) / 1024)
	return
}

func getWindowsUptime() int64 {
	out, err := runPS(`(Get-Date) - (gcim Win32_OperatingSystem).LastBootUpTime | Select-Object -ExpandProperty TotalSeconds`)
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(out), 64)
	return int64(v)
}

func getWindowsNetworkMetrics() NetworkMetrics {
	// Read first non-loopback adapter counters. Two samples 1s apart → delta bytes/s.
	sample := func() (string, uint64, uint64) {
		out, err := runPS(`
			$a = Get-NetAdapterStatistics | Where-Object { $_.Name -notmatch 'Loopback' } | Select-Object -First 1
			if ($a) { "$($a.Name) $($a.ReceivedBytes) $($a.SentBytes)" }
		`)
		if err != nil {
			return "", 0, 0
		}
		parts := strings.Fields(strings.TrimSpace(out))
		if len(parts) < 3 {
			return "", 0, 0
		}
		rx, _ := strconv.ParseUint(parts[1], 10, 64)
		tx, _ := strconv.ParseUint(parts[2], 10, 64)
		return parts[0], rx, tx
	}

	name, rx1, tx1 := sample()
	time.Sleep(1 * time.Second)
	_, rx2, tx2 := sample()

	return NetworkMetrics{
		Interface: name,
		RXBytes:   rx2 - rx1,
		TXBytes:   tx2 - tx1,
	}
}

func getWindowsBattery() BatteryMetrics {
	out, err := runPS(`
		$b = Get-CimInstance Win32_Battery | Select-Object -First 1
		if ($b) { "$($b.EstimatedChargeRemaining) $($b.BatteryStatus)" }
	`)
	if err != nil || strings.TrimSpace(out) == "" {
		return BatteryMetrics{Capacity: 100, Status: "AC"}
	}
	parts := strings.Fields(strings.TrimSpace(out))
	if len(parts) < 2 {
		return BatteryMetrics{Capacity: 100, Status: "AC"}
	}
	cap, _ := strconv.Atoi(parts[0])
	// BatteryStatus: 1=Other, 2=Unknown, 3=Fully Charged, 4=Low, 5=Critical,
	//                6=Charging, 7=Charging+High, 8=Charging+Low, 9=Charging+Crit, 11=Partially Charged
	statusMap := map[string]string{
		"3": "Full", "6": "Charging", "7": "Charging", "8": "Charging",
		"9": "Charging", "11": "Discharging", "4": "Low", "5": "Critical",
	}
	status := statusMap[parts[1]]
	if status == "" {
		status = "Unknown"
	}
	return BatteryMetrics{Capacity: cap, Status: status}
}

// runPS executes a PowerShell snippet and returns trimmed stdout.
func runPS(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return strings.TrimSpace(sb.String()), nil
}
