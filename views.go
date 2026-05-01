package main

import (
	"fmt"

	"github.com/FreyreCorona/SideCar/metrics"
)

type (
	TextBlock struct {
		Text  string `json:"text"`
		Size  int    `json:"size"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Color string `json:"color"` // hex
	}

	ImageBlock struct {
		Path   string `json:"path"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}

	RenderFrame struct {
		Texts  []TextBlock  `json:"texts"`
		Images []ImageBlock `json:"images"`
	}
)

var currentView = 0
var onViewChange func()

func getCurrentFrame() RenderFrame {
	switch currentView {
	case 0:
		return CPUAndMemoryView()
	case 1:
		return NetworkView()
	default:
		return PowerView()
	}
}

func nextView() {
	currentView++
	if currentView > 2 {
		currentView = 0
	}
}

func setView(v int) {
	currentView = v
}

func CPUAndMemoryView() RenderFrame {
	cpu := metrics.CollectCPUMetrics()
	mem := metrics.CollectMemoryMetrics()

	return RenderFrame{
		Texts: []TextBlock{
			{Text: "CPU", Size: 14, X: 20, Y: 20, Color: "#a1a1aa"},
			{Text: fmt.Sprintf("%.1f%%", cpu.UsagePercent), Size: 48, X: 20, Y: 40, Color: "#6366f1"},
			{Text: fmt.Sprintf("%.1f°C", cpu.Temperature), Size: 18, X: 20, Y: 100, Color: "#ef4444"},

			{Text: "MEMORY", Size: 14, X: 20, Y: 140, Color: "#a1a1aa"},
			{Text: fmt.Sprintf("%d MB", mem.UsedMB), Size: 32, X: 20, Y: 160, Color: "#a855f7"},
			{Text: fmt.Sprintf("of %d MB", mem.TotalMB), Size: 12, X: 20, Y: 200, Color: "#52525b"},
		},
	}
}

func NetworkView() RenderFrame {
	net := metrics.CollectNetworkMetrics()

	wifiIcon := "[=]" // ASCII icon for wired connection
	if net.WifiSSID != "" {
		wifiIcon = "[~]" // ASCII icon for WiFi
	}

	return RenderFrame{
		Texts: []TextBlock{
			{Text: "NETWORK", Size: 14, X: 20, Y: 20, Color: "#a1a1aa"},
			{Text: net.Interface, Size: 24, X: 20, Y: 40, Color: "#10b981"},

			{Text: wifiIcon + " " + net.WifiSSID, Size: 18, X: 20, Y: 80, Color: "#fafafa"},
			{Text: fmt.Sprintf("Quality: %d%%", net.WifiQuality), Size: 14, X: 20, Y: 105, Color: "#a1a1aa"},

			{Text: "↑ " + fmtBytes(net.TXBytes), Size: 16, X: 20, Y: 140, Color: "#6366f1"},
			{Text: "↓ " + fmtBytes(net.RXBytes), Size: 16, X: 20, Y: 165, Color: "#a855f7"},
		},
	}
}

func PowerView() RenderFrame {
	bat := metrics.CollectBatteryMetrics()
	up := metrics.CollectUptimeMetrics()

	return RenderFrame{
		Texts: []TextBlock{
			{Text: "BATTERY", Size: 14, X: 20, Y: 20, Color: "#a1a1aa"},
			{Text: fmt.Sprintf("%d%%", bat.Capacity), Size: 54, X: 20, Y: 40, Color: "#10b981"},
			{Text: bat.Status, Size: 16, X: 20, Y: 105, Color: "#a1a1aa"},

			{Text: "UPTIME", Size: 14, X: 20, Y: 145, Color: "#a1a1aa"},
			{Text: fmtUptime(up.Seconds), Size: 24, X: 20, Y: 165, Color: "#f59e0b"},
		},
	}
}

func fmtBytes(b uint64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

func fmtUptime(s int64) string {
	h := s / 3600
	m := (s % 3600) / 60
	return fmt.Sprintf("%dh %dm", h, m)
}
