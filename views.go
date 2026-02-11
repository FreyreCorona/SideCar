package main

import (
	"fmt"

	"github.com/FreyreCorona/SideCar/metrics"
)

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
