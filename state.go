package main

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
