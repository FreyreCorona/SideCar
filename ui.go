package main

import (
	"log"
	"net/http"

	wv "github.com/webview/webview_go"
)

func runUI() error {
	debug := true
	w := wv.New(debug)
	defer w.Destroy()

	w.SetTitle("Vision R15 Controller")
	w.SetSize(900, 600, wv.HintNone)

	// Servir UI
	fs := http.FileServer(http.Dir("ui"))
	http.Handle("/", fs)

	go func() {
		log.Println("UI server on http://127.0.0.1:8080")
		if err := http.ListenAndServe("127.0.0.1:8080", nil); err != nil {
			log.Fatal(err)
		}
	}()

	// API UI → Go
	w.Bind("setImage", func(payload string) {
		log.Println("Set image:", payload)
		// aquí luego llamas a core/
	})

	w.Navigate("http://127.0.0.1:8080/index.html")
	w.Run()

	return nil
}
