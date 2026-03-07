package tests

import (
	"testing"

	"github.com/FreyreCorona/SideCar/core"
)

// ── BatteryLevel ───────────────────────────────────────────────────────────

func TestBatteryLevel(t *testing.T) {
	tests := []struct {
		pct  int
		want uint32
	}{
		{0, 0},
		{19, 0},
		{20, 1},
		{39, 1},
		{40, 2},
		{59, 2},
		{60, 3},
		{79, 3},
		{80, 4},
		{99, 4},
		{100, 5},
		{150, 5}, // over 100% edge case
	}
	for _, tc := range tests {
		got := core.BatteryLevel(tc.pct)
		if got != tc.want {
			t.Errorf("BatteryLevel(%d) = %d, want %d", tc.pct, got, tc.want)
		}
	}
}

func TestBatteryLevel_Boundaries(t *testing.T) {
	// Check that every level boundary is strict (< not <=)
	if core.BatteryLevel(20) == core.BatteryLevel(19) {
		t.Error("boundary at 20% should change the level")
	}
	if core.BatteryLevel(40) == core.BatteryLevel(39) {
		t.Error("boundary at 40% should change the level")
	}
}

// ── WifiQualityLevel ───────────────────────────────────────────────────────

func TestWifiQualityLevel(t *testing.T) {
	tests := []struct {
		pct  int
		want uint32
	}{
		{0, 0},
		{30, 0},
		{31, 1},
		{50, 1},
		{51, 2},
		{69, 2},
		{70, 3},
		{100, 3},
	}
	for _, tc := range tests {
		got := core.WifiQualityLevel(tc.pct)
		if got != tc.want {
			t.Errorf("WifiQualityLevel(%d) = %d, want %d", tc.pct, got, tc.want)
		}
	}
}

func TestWifiQualityLevel_ReturnRange(t *testing.T) {
	for pct := 0; pct <= 100; pct++ {
		got := core.WifiQualityLevel(pct)
		if got > 3 {
			t.Errorf("WifiQualityLevel(%d) = %d, must be 0–3", pct, got)
		}
	}
}

// ── WeatherType ────────────────────────────────────────────────────────────

func TestWeatherType(t *testing.T) {
	tests := []struct {
		main string
		want uint32
	}{
		{"Clear", 1},
		{"Clouds", 2},
		{"Mist", 2},
		{"Fog", 2},
		{"Haze", 2},
		{"Dust", 2},
		{"Smoke", 2},
		{"Sand", 2},
		{"Ash", 2},
		{"Squall", 2},
		{"Tornado", 2},
		{"Rain", 4},
		{"Drizzle", 4},
		{"Thunderstorm", 4},
		{"Snow", 4},
		{"Unknown", 0},
		{"", 0},
		{"CLEAR", 0}, // case-sensitive
	}
	for _, tc := range tests {
		got := core.WeatherType(tc.main)
		if got != tc.want {
			t.Errorf("WeatherType(%q) = %d, want %d", tc.main, got, tc.want)
		}
	}
}

// ── Register constants sanity checks ──────────────────────────────────────

func TestRegisterConstants_Unique(t *testing.T) {
	// Verify that critical system registers don't overlap
	system := map[string]uint16{
		"RegDate":        core.RegDate,
		"RegTime":        core.RegTime,
		"RegBrightness":  core.RegBrightness,
		"RegCPU0Version": core.RegCPU0Version,
		"RegCPU1Version": core.RegCPU1Version,
		"RegCurrentPage": core.RegCurrentPage,
	}
	seen := make(map[uint16]string)
	for name, id := range system {
		if prev, exists := seen[id]; exists {
			t.Errorf("register ID collision: %s and %s both have ID %d", name, prev, id)
		}
		seen[id] = name
	}
}

func TestRegisterConstants_DataRange(t *testing.T) {
	// All data registers should be in the 1080–2003 range per tagNameMap
	dataRegs := map[string]uint16{
		"RegCPUUsage":       core.RegCPUUsage,
		"RegGPUUsage":       core.RegGPUUsage,
		"RegBatteryPercent": core.RegBatteryPercent,
		"RegWifiSSID":       core.RegWifiSSID,
		"RegWifiQuality":    core.RegWifiQuality,
		"RegBTName":         core.RegBTName,
		"RegBTStatus":       core.RegBTStatus,
		"RegWifiStatus":     core.RegWifiStatus,
		"RegBatteryType":    core.RegBatteryType,
	}
	for name, id := range dataRegs {
		if id < 1080 || id > 2100 {
			t.Errorf("%s = %d, expected in range 1080–2100", name, id)
		}
	}
}
