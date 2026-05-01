//go:build linux

package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// SIOCGIWESSID is the ioctl code to get the ESSID in Linux (Wireless Extensions)
const SIOCGIWESSID = 0x8B1B

func CollectCPUMetrics() CPUMetrics {
	return CPUMetrics{
		UsagePercent: getCPUUsage(),
		Temperature:  getCPUTemperature(),
	}
}

func CollectMemoryMetrics() MemoryMetrics {
	used, total := getMemoryUsage()
	return MemoryMetrics{
		UsedMB:  used,
		TotalMB: total,
	}
}

func CollectUptimeMetrics() UptimeMetrics {
	return UptimeMetrics{
		Seconds: getUptime(),
	}
}

func CollectNetworkMetrics() NetworkMetrics {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetworkMetrics{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var nm NetworkMetrics

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.Split(line, ":")
		iface := strings.TrimSpace(parts[0])

		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)

		nm = NetworkMetrics{
			Interface: iface,
			RXBytes:   rx,
			TXBytes:   tx,
		}
		// Try to get WiFi info for this interface
		ssid, quality := getWifiInfo(iface)
		if ssid != "" || quality > 0 {
			nm.WifiSSID = ssid
			nm.WifiQuality = quality
			break // If we found an interface with WiFi, we stick with that one
		}
	}

	return nm
}

// getWifiInfo retrieves SSID and quality without using external commands
func getWifiInfo(iface string) (string, int) {
	if iface == "" {
		return "", 0
	}

	quality := 0
	// 1. Get Quality from /proc/net/wireless (Safe file reading)
	wfile, err := os.Open("/proc/net/wireless")
	if err == nil {
		defer wfile.Close()
		wscanner := bufio.NewScanner(wfile)
		for wscanner.Scan() {
			wline := wscanner.Text()
			if strings.Contains(wline, iface+":") {
				wfields := strings.Fields(wline)
				if len(wfields) >= 3 {
					// Field 3 is link quality (usually 0-70 or 0-100)
					qStr := strings.TrimSuffix(wfields[2], ".")
					q, _ := strconv.ParseFloat(qStr, 64)
					// Normalize to 0-100 (WEXT usually uses 70 as max)
					if q > 0 && q <= 70 {
						quality = int(q * 100 / 70)
					} else {
						quality = int(q)
					}
				}
			}
		}
	}

	// 2. Get SSID via ioctl (Native system call)
	ssid := getSSIDNative(iface)

	return ssid, quality
}

// getSSIDNative uses ioctl to talk directly to the network driver
func getSSIDNative(iface string) string {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return ""
	}
	defer syscall.Close(fd)

	// iwreq structure for SIOCGIWESSID
	var essid [32]byte
	var req struct {
		name [16]byte
		ptr  uintptr
		len  uint16
		flg  uint16
		pad  [4]byte // Padding for 64-bit alignment
	}

	copy(req.name[:], iface)
	req.ptr = uintptr(unsafe.Pointer(&essid[0]))
	req.len = uint16(len(essid))

	// Direct ioctl call
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), SIOCGIWESSID, uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		return ""
	}

	return strings.TrimRight(string(essid[:req.len]), "\x00")
}

func CollectBatteryMetrics() BatteryMetrics {
	// Buscar cualquier batería disponible (BAT0, BAT1, etc.)
	base := "/sys/class/power_supply/"
	entries, err := os.ReadDir(base)
	if err != nil {
		return BatteryMetrics{}
	}

	for _, entry := range entries {
		name := entry.Name()
		if len(name) >= 3 && name[:3] == "BAT" {
			capData, err1 := os.ReadFile(base + name + "/capacity")
			statusData, err2 := os.ReadFile(base + name + "/status")
			if err1 != nil || err2 != nil {
				continue
			}
			capacity, _ := strconv.Atoi(strings.TrimSpace(string(capData)))
			status := strings.TrimSpace(string(statusData))
			return BatteryMetrics{
				Capacity: capacity,
				Status:   status,
			}
		}
	}
	return BatteryMetrics{}
}

type cpuSample struct {
	idle  uint64
	total uint64
}

func readCPUSample() (cpuSample, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	fields := strings.Fields(scanner.Text())

	var total uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
	}
	idle, _ := strconv.ParseUint(fields[4], 10, 64)

	return cpuSample{idle: idle, total: total}, nil
}

func getCPUUsage() float64 {
	s1, err := readCPUSample()
	if err != nil {
		return 0
	}
	time.Sleep(200 * time.Millisecond)
	s2, err := readCPUSample()
	if err != nil {
		return 0
	}

	deltaIdle := s2.idle - s1.idle
	deltaTotal := s2.total - s1.total
	if deltaTotal == 0 {
		return 0
	}
	return float64(deltaTotal-deltaIdle) / float64(deltaTotal) * 100
}

func getMemoryUsage() (usedMB int, totalMB int) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer file.Close()

	var total, available int
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if fields[0] == "MemTotal:" {
			val, _ := strconv.Atoi(fields[1])
			total = val
		}
		if fields[0] == "MemAvailable:" {
			val, _ := strconv.Atoi(fields[1])
			available = val
		}
	}
	totalMB = total / 1024
	usedMB = (total - available) / 1024
	return
}

func getUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func getCPUTemperature() float64 {
	// Buscar en todas las zonas térmica hasta encontrar una con temperatura válida
	base := "/sys/class/thermal/"
	entries, err := os.ReadDir(base)
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		data, err := os.ReadFile(base + entry.Name() + "/temp")
		if err != nil {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			continue
		}
		if value > 0 {
			return float64(value) / 1000.0
		}
	}
	return 0
}
