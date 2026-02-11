package main

func SystemStatsView() RenderFrame {
	return RenderFrame{
		Blocks: []TextBlock{
			{Text: "CPU Usage: 45%", Size: 24},
			{Text: "Memory Usage: 3.2 GB", Size: 24},
			{Text: "Disk Usage: 120 GB", Size: 24},
		},
	}
}
