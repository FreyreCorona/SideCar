package main

func SystemStatsView() RenderFrame {
	return RenderFrame{
		Texts: []TextBlock{
			{Text: "CPU Usage: 45%", Size: 24},
			{Text: "Memory Usage: 3.2 GB", Size: 24},
			{Text: "Disk Usage: 120 GB", Size: 24},
		},
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
