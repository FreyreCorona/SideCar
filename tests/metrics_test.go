package tests

import (
	"testing"

	"github.com/FreyreCorona/SideCar/metrics"
)

// ── CPUMetrics ─────────────────────────────────────────────────────────────

func TestCPUMetrics_Fields(t *testing.T) {
	m := metrics.CPUMetrics{UsagePercent: 55.5, Temperature: 72.3}
	if m.UsagePercent != 55.5 {
		t.Errorf("UsagePercent = %v, want 55.5", m.UsagePercent)
	}
	if m.Temperature != 72.3 {
		t.Errorf("Temperature = %v, want 72.3", m.Temperature)
	}
}

func TestCPUMetrics_ZeroValue(t *testing.T) {
	var m metrics.CPUMetrics
	if m.UsagePercent != 0 || m.Temperature != 0 {
		t.Error("zero-value CPUMetrics should have all fields at 0")
	}
}

// ── MemoryMetrics ──────────────────────────────────────────────────────────

func TestMemoryMetrics_Fields(t *testing.T) {
	m := metrics.MemoryMetrics{UsedMB: 4096, TotalMB: 16384}
	if m.UsedMB != 4096 {
		t.Errorf("UsedMB = %d, want 4096", m.UsedMB)
	}
	if m.TotalMB != 16384 {
		t.Errorf("TotalMB = %d, want 16384", m.TotalMB)
	}
}

func TestMemoryMetrics_UsedCannotExceedTotal(t *testing.T) {
	// Semantic invariant: a sane collector should not report used > total
	m := metrics.MemoryMetrics{UsedMB: 8192, TotalMB: 16384}
	if m.UsedMB > m.TotalMB {
		t.Errorf("UsedMB (%d) > TotalMB (%d)", m.UsedMB, m.TotalMB)
	}
}

// ── NetworkMetrics ─────────────────────────────────────────────────────────

func TestNetworkMetrics_Fields(t *testing.T) {
	m := metrics.NetworkMetrics{
		Interface: "eth0",
		RXBytes:   1024 * 1024,
		TXBytes:   512 * 1024,
	}
	if m.Interface != "eth0" {
		t.Errorf("Interface = %q, want eth0", m.Interface)
	}
	if m.RXBytes != 1024*1024 {
		t.Errorf("RXBytes = %d, want %d", m.RXBytes, 1024*1024)
	}
}

func TestNetworkMetrics_ZeroValue(t *testing.T) {
	var m metrics.NetworkMetrics
	if m.Interface != "" || m.RXBytes != 0 || m.TXBytes != 0 {
		t.Error("zero-value NetworkMetrics should have empty fields")
	}
}

// ── BatteryMetrics ─────────────────────────────────────────────────────────

func TestBatteryMetrics_Fields(t *testing.T) {
	m := metrics.BatteryMetrics{Capacity: 85, Status: "Discharging"}
	if m.Capacity != 85 {
		t.Errorf("Capacity = %d, want 85", m.Capacity)
	}
	if m.Status != "Discharging" {
		t.Errorf("Status = %q, want Discharging", m.Status)
	}
}

func TestBatteryMetrics_CapacityRange(t *testing.T) {
	for _, c := range []int{0, 50, 100} {
		m := metrics.BatteryMetrics{Capacity: c}
		if m.Capacity < 0 || m.Capacity > 100 {
			t.Errorf("Capacity %d out of range 0–100", m.Capacity)
		}
	}
}

// ── UptimeMetrics ──────────────────────────────────────────────────────────

func TestUptimeMetrics_Fields(t *testing.T) {
	m := metrics.UptimeMetrics{Seconds: 3600}
	if m.Seconds != 3600 {
		t.Errorf("Seconds = %d, want 3600", m.Seconds)
	}
}

func TestUptimeMetrics_NonNegative(t *testing.T) {
	m := metrics.UptimeMetrics{Seconds: 0}
	if m.Seconds < 0 {
		t.Error("uptime Seconds must be non-negative")
	}
}

// ── CollectCPUMetrics (smoke test — doesn't require hardware) ──────────────

func TestCollectCPUMetrics_ReturnsWithoutPanic(t *testing.T) {
	// Just verify it doesn't panic and returns plausible values
	m := metrics.CollectCPUMetrics()
	if m.UsagePercent < 0 || m.UsagePercent > 100 {
		t.Errorf("UsagePercent %v out of range 0–100", m.UsagePercent)
	}
}

func TestCollectMemoryMetrics_Consistent(t *testing.T) {
	m := metrics.CollectMemoryMetrics()
	if m.TotalMB < 0 {
		t.Error("TotalMB must be non-negative")
	}
	if m.UsedMB < 0 {
		t.Error("UsedMB must be non-negative")
	}
	// If TotalMB > 0, UsedMB should not exceed it
	if m.TotalMB > 0 && m.UsedMB > m.TotalMB {
		t.Errorf("UsedMB (%d) > TotalMB (%d)", m.UsedMB, m.TotalMB)
	}
}

func TestCollectBatteryMetrics_CapacityRange(t *testing.T) {
	m := metrics.CollectBatteryMetrics()
	if m.Capacity < 0 || m.Capacity > 100 {
		t.Errorf("Battery capacity %d out of 0–100 range", m.Capacity)
	}
}

func TestCollectNetworkMetrics_ReturnsWithoutPanic(t *testing.T) {
	_ = metrics.CollectNetworkMetrics()
}

func TestCollectUptimeMetrics_NonNegative(t *testing.T) {
	m := metrics.CollectUptimeMetrics()
	if m.Seconds < 0 {
		t.Errorf("uptime %d must be non-negative", m.Seconds)
	}
}
