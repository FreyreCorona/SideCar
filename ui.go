package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/FreyreCorona/SideCar/core"
	"github.com/FreyreCorona/SideCar/metrics"

	wv "github.com/abemedia/go-webview"
	_ "github.com/abemedia/go-webview/embedded"
)

// ── Estado global de la UI ───────────────────────────────────

type UIState struct {
	mu          sync.Mutex
	dev         *core.Device
	daemonCtx   context.Context
	daemonCancel context.CancelFunc
	win         wv.WebView
	uploadProg  func(int)
}

var uiState UIState

// ── Entry point ───────────────────────────────────────────────

func runUI() error {
	debug := os.Getenv("SIDECAR_DEBUG") == "1"
	w := wv.New(debug)
	defer w.Destroy()

	uiState.win = w
	uiState.uploadProg = func(pct int) {
		w.Dispatch(func() {
			w.Eval(fmt.Sprintf(`window.onUploadProgress && window.onUploadProgress(%d)`, pct))
		})
	}

	w.SetTitle("SideCar")
	w.SetSize(900, 580, wv.HintNone)

	// ── HTTP server para servir los archivos de UI ─────────────
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("ui")))

	mux.HandleFunc("/statics/", func(wr http.ResponseWriter, r *http.Request) {
		rel := r.URL.Path[len("/statics/"):]
		http.ServeFile(wr, r, filepath.Join("ui", "statics", rel))
	})

	server := &http.Server{Addr: "127.0.0.1:8481", Handler: mux}
	go func() {
		log.Println("UI server: http://127.0.0.1:8481")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// ── Bindings Go → JS ──────────────────────────────────────

	// getCurrentFrame: retorna el frame de la vista actual para el canvas
	w.Bind("getCurrentFrame", getCurrentFrame)

	// nextView / setView
	w.Bind("nextView", func() {
		nextView()
		if onViewChange != nil {
			w.Dispatch(func() { w.Eval(`window.onViewChanged && window.onViewChanged()`) })
		}
	})
	w.Bind("setView", func(idx int) {
		if idx >= 0 {
			currentView = idx % 3
		}
	})

	// startDaemon: conecta al device e inicia el loop de métricas
	w.Bind("startDaemon", func() bool {
		uiState.mu.Lock()
		defer uiState.mu.Unlock()

		if uiState.daemonCancel != nil {
			// ya corriendo
			return true
		}

		dev, err := core.AutoConnect(115200)
		if err != nil {
			log.Println("startDaemon: connect:", err)
			return false
		}
		uiState.dev = dev

		ctx, cancel := context.WithCancel(context.Background())
		uiState.daemonCtx = ctx
		uiState.daemonCancel = cancel

		go RunDaemon(ctx, DaemonConfig{
			Device:   "auto",
			Baud:     115200,
			Interval: 5 * time.Second,
			Log:      io.Discard,
		})

		return true
	})

	// stopDaemon: detiene el daemon
	w.Bind("stopDaemon", func() {
		uiState.mu.Lock()
		defer uiState.mu.Unlock()
		if uiState.daemonCancel != nil {
			uiState.daemonCancel()
			uiState.daemonCancel = nil
		}
		if uiState.dev != nil {
			uiState.dev.Close()
			uiState.dev = nil
		}
	})

	// getStats: retorna métricas actuales
	w.Bind("getStats", func() map[string]interface{} {
		cpu := metrics.CollectCPUMetrics()
		mem := metrics.CollectMemoryMetrics()
		bat := metrics.CollectBatteryMetrics()
		net := metrics.CollectNetworkMetrics()
		up  := metrics.CollectUptimeMetrics()

		ramPct := 0
		if mem.TotalMB > 0 {
			ramPct = 100 * mem.UsedMB / mem.TotalMB
		}

		return map[string]interface{}{
			"cpu":      cpu.UsagePercent,
			"temp":     cpu.Temperature,
			"ramUsed":  mem.UsedMB,
			"ramTotal": mem.TotalMB,
			"ramPct":   ramPct,
			"battery":  bat.Capacity,
			"batStatus": bat.Status,
			"netIface": net.Interface,
			"rxBytes":  net.RXBytes,
			"txBytes":  net.TXBytes,
			"uptime":   up.Seconds,
		}
	})

	// setBrightness
	w.Bind("setBrightness", func(level int) {
		uiState.mu.Lock()
		dev := uiState.dev
		uiState.mu.Unlock()
		if dev == nil {
			return
		}
		if level < 0 {
			level = 0
		}
		if level > 255 {
			level = 255
		}
		if err := dev.SetBrightness(uint8(level)); err != nil {
			log.Println("setBrightness:", err)
		}
	})

	// wake / sleep / reboot
	w.Bind("wake", func() {
		withDev(func(d *core.Device) { d.Wake() })
	})
	w.Bind("sleep", func() {
		withDev(func(d *core.Device) { d.Sleep() })
	})
	w.Bind("reboot", func() {
		withDev(func(d *core.Device) { d.Reboot() })
	})

	// showPage
	w.Bind("showPage", func(page int) {
		withDev(func(d *core.Device) {
			d.WriteNumRegisters([]core.NumRegister{
				{RegID: core.RegCurrentPage, Value: uint32(page)},
			})
		})
	})

	// uploadFile: recibe []int (bytes) desde JS y sube al device
	w.Bind("uploadFile", func(bytesArr []int, fileType string) map[string]interface{} {
		uiState.mu.Lock()
		dev := uiState.dev
		uiState.mu.Unlock()

		if dev == nil {
			return map[string]interface{}{"error": "device not connected"}
		}

		data := make([]byte, len(bytesArr))
		for i, b := range bytesArr {
			data[i] = byte(b)
		}

		ft := core.FileType(fileType)
		cfg := core.UploadConfig{
			OnProgress: func(p core.UploadProgress) {
				if uiState.uploadProg != nil {
					uiState.uploadProg(p.Percent)
				}
			},
		}

		if err := dev.UploadFile(data, ft, cfg); err != nil {
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{"ok": true}
	})

	// listPorts: retorna los puertos seriales disponibles
	w.Bind("listPorts", func() []string {
		ports, err := core.FindSerialDevices()
		if err != nil {
			return []string{}
		}
		return ports
	})

	// setImage: manejo de imagen personalizada (futuro)
	w.Bind("setImage", func(payload string) {
		var p struct {
			Asset string `json:"asset"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			log.Println("setImage: parse:", err)
		}
		log.Println("setImage:", p.Asset)
		// TODO: conectar con upload de ACF personalizado
	})

	// onViewChange hook
	onViewChange = func() {
		w.Dispatch(func() {
			w.Eval(`window.onViewChanged && window.onViewChanged()`)
		})
	}

	w.Navigate("http://127.0.0.1:8481")
	w.Run()
	return nil
}

// ── Helpers ───────────────────────────────────────────────────

func withDev(fn func(*core.Device)) {
	uiState.mu.Lock()
	dev := uiState.dev
	uiState.mu.Unlock()
	if dev == nil {
		log.Println("withDev: no device connected")
		return
	}
	fn(dev)
}
