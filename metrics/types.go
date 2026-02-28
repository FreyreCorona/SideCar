package metrics

// CPUMetrics holds CPU usage and temperature data.
type CPUMetrics struct {
	UsagePercent float64
	Temperature  float64
}

// MemoryMetrics holds RAM usage data in megabytes.
type MemoryMetrics struct {
	UsedMB  int
	TotalMB int
}

// UptimeMetrics holds system uptime in seconds.
type UptimeMetrics struct {
	Seconds int64
}

// NetworkMetrics holds per-interface network counters.
type NetworkMetrics struct {
	Interface string
	RXBytes   uint64
	TXBytes   uint64
}

// BatteryMetrics holds battery capacity and charge status.
type BatteryMetrics struct {
	Capacity int
	Status   string
}
