package metrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

type CPUMetrics struct {
	UsagePercent float64
	Temperature  float64
}

func CollectCPUMetrics() CPUMetrics {
	return CPUMetrics{
		UsagePercent: getCPUUsage(),
		Temperature:  getCPUTemperature(),
	}
}

type MemoryMetrics struct {
	UsedMB  int
	TotalMB int
}

func CollectMemoryMetrics() MemoryMetrics {
	used, total := getMemoryUsage()
	return MemoryMetrics{
		UsedMB:  used,
		TotalMB: total,
	}
}

type UptimeMetrics struct {
	Seconds int64
}

func CollectUptimeMetrics() UptimeMetrics {
	return UptimeMetrics{
		Seconds: getUptime(),
	}
}

type NetworkMetrics struct {
	Interface string
	RXBytes   uint64
	TXBytes   uint64
}

func CollectNetworkMetrics() NetworkMetrics {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return NetworkMetrics{}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.Contains(line, ":") {
			continue
		}

		parts := strings.Split(line, ":")
		iface := strings.TrimSpace(parts[0])

		// ignorar loopback
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])

		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)

		return NetworkMetrics{
			Interface: iface,
			RXBytes:   rx,
			TXBytes:   tx,
		}
	}

	return NetworkMetrics{}
}

type BatteryMetrics struct {
	Capacity int
	Status   string
}

func CollectBatteryMetrics() BatteryMetrics {
	base := "/sys/class/power_supply/BAT0/"

	capData, _ := os.ReadFile(base + "capacity")
	statusData, _ := os.ReadFile(base + "status")

	capacity, _ := strconv.Atoi(strings.TrimSpace(string(capData)))
	status := strings.TrimSpace(string(statusData))

	return BatteryMetrics{
		Capacity: capacity,
		Status:   status,
	}
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
	// fields[0] == "cpu"

	var total uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
	}

	idle, _ := strconv.ParseUint(fields[4], 10, 64)

	return cpuSample{
		idle:  idle,
		total: total,
	}, nil
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
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0
	}

	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	return float64(value) / 1000.0
}
