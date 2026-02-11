package main

var currentView = 0

func getCurrentFrame() RenderFrame {
	switch currentView {
	case 0:
		return SystemStatsView()
	case 1:
		return ImageView()
	default:
		return SystemStatsView()
	}
}

func nextView() {
	currentView++
	if currentView > 1 {
		currentView = 0
	}
}
