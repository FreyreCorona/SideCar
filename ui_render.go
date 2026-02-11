package main

type TextBlock struct {
	Text string `json:"text"`
	Size int    `json:"size"`
}

type RenderFrame struct {
	Blocks []TextBlock `json:"blocks"`
}
