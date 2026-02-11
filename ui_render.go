package main

type TextBlock struct {
	Text string `json:"text"`
	Size int    `json:"size"`
}

type ImageBlock struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type RenderFrame struct {
	Texts  []TextBlock  `json:"texts"`
	Images []ImageBlock `json:"images"`
}
