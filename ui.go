package main

import (
	"log"
	"net/http"

	wv "github.com/abemedia/go-webview"
	_ "github.com/abemedia/go-webview/embedded"
)

func runUI() error {
	debug := true
	w := wv.New(debug)
	defer w.Destroy()

	w.SetTitle("Side Car Controller")
	w.SetSize(1000, 650, wv.HintNone)

	// Servir UI
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("ui")))

	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	go func() {
		log.Println("UI server on http://127.0.0.1:8080")
		if err := server.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	w.Bind("setImage", func(payload string) {
		log.Println("Set image:", payload)
		// aquí luego llamas a core/
	})

	w.Bind("getCurrentFrame", func() RenderFrame {
		log.Println("Getting Stats")
		return SystemStatsView()
	})

	w.Navigate("http://127.0.0.1:8080")
	w.Run()

	return nil
}
