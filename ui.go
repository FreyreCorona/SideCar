package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

	mux.HandleFunc("/upload", func(wr http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(wr, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			http.Error(wr, "Bad request", http.StatusBadRequest)
			return
		}
		defer file.Close()

		os.MkdirAll("ui/statics", 0755)
		dstPath := filepath.Join("ui/statics", header.Filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			http.Error(wr, "Failed to save", http.StatusInternalServerError)
			return
		}
		defer dst.Close()
		io.Copy(dst, file)

		// Devuelve la ruta relativa para el frontend
		wr.Header().Set("Content-Type", "application/json")
		io.WriteString(wr, `{"path":"/statics/`+header.Filename+`"}`)
	})

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

	w.Bind("getCurrentFrame", getCurrentFrame)
	w.Bind("nextView", nextView)

	onViewChange = func() {
		w.Dispatch(func() {
			w.Eval(`window.onViewChanged()`)
		})
	}

	w.Navigate("http://127.0.0.1:8080")
	w.Run()

	return nil
}
