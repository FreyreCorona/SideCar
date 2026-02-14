package main

import (
	"fmt"

	"github.com/FreyreCorona/SideCar/metrics"
)

type (
	TextBlock struct {
		Text string `json:"text"`
		Size int    `json:"size"`
	}

	ImageBlock struct {
		Path   string `json:"path"`
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
	if currentView > 1 {
		currentView = 0
	}
}

func ImageView() RenderFrame {
	return RenderFrame{
		Texts: []TextBlock{
			{Text: "Wallpaper Mode", Size: 40},
		},
		Images: []ImageBlock{
			{
				Path:   "statics/sample.jpg",
				Width:  800,
				Height: 600,
			},
		},
	}
}

func CPUAndMemoryView() RenderFrame {
	cpu := metrics.CollectCPUMetrics()
	mem := metrics.CollectMemoryMetrics()

	return RenderFrame{
		Texts: []TextBlock{
			{Text: fmt.Sprintf("CPU: %.1f%%", cpu.UsagePercent), Size: 26},
			{Text: fmt.Sprintf("Temp: %.1f°C", cpu.Temperature), Size: 22},
			{Text: fmt.Sprintf("RAM: %d / %d MB", mem.UsedMB, mem.TotalMB), Size: 26},
		},
	}
}

func NetworkView() RenderFrame {
	net := metrics.CollectNetworkMetrics()

	return RenderFrame{
		Texts: []TextBlock{
			{Text: "Interface: " + net.Interface, Size: 24},
			{Text: fmt.Sprintf("RX: %d bytes", net.RXBytes), Size: 22},
			{Text: fmt.Sprintf("TX: %d bytes", net.TXBytes), Size: 22},
		},
	}
}

func PowerView() RenderFrame {
	bat := metrics.CollectBatteryMetrics()
	up := metrics.CollectUptimeMetrics()

	return RenderFrame{
		Texts: []TextBlock{
			{Text: fmt.Sprintf("Battery: %d%%", bat.Capacity), Size: 28},
			{Text: "Status: " + bat.Status, Size: 22},
			{Text: fmt.Sprintf("Uptime: %ds", up.Seconds), Size: 20},
		},
	}
}
