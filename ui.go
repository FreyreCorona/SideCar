package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
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

// ── Global UI state ───────────────────────────────────────────

type UIState struct {
	mu           sync.Mutex
	dev          *core.Device
	daemonCtx    context.Context
	daemonCancel context.CancelFunc
	win          wv.WebView
	uploadProg   func(int)
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
	w.SetSize(960, 620, wv.HintNone)

	// ── HTTP server ────────────────────────────────────────────
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

	// ── Go → JS bindings ──────────────────────────────────────

	w.Bind("getCurrentFrame", getCurrentFrame)

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

	// startDaemon: connects to the device and starts the metrics loop using
	// the SAME connection — no double-connect.
	w.Bind("startDaemon", func() bool {
		uiState.mu.Lock()
		defer uiState.mu.Unlock()

		if uiState.daemonCancel != nil {
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

		go runUIMetricsLoop(ctx, dev, w)

		return true
	})

	// stopDaemon: stops the daemon and closes the device.
	w.Bind("stopDaemon", func() {
		uiState.mu.Lock()
		defer uiState.mu.Unlock()

		if uiState.daemonCancel != nil {
			uiState.daemonCancel()
			uiState.daemonCancel = nil
			uiState.daemonCtx = nil
		}
		if uiState.dev != nil {
			uiState.dev.Close()
			uiState.dev = nil
		}
	})

	// getStats: reads system metrics (does not require a device)
	w.Bind("getStats", func() map[string]interface{} {
		cpu := metrics.CollectCPUMetrics()
		mem := metrics.CollectMemoryMetrics()
		bat := metrics.CollectBatteryMetrics()
		net := metrics.CollectNetworkMetrics()
		up := metrics.CollectUptimeMetrics()

		ramPct := 0
		if mem.TotalMB > 0 {
			ramPct = 100 * mem.UsedMB / mem.TotalMB
		}

		return map[string]interface{}{
			"cpu":       cpu.UsagePercent,
			"temp":      cpu.Temperature,
			"ramUsed":   mem.UsedMB,
			"ramTotal":  mem.TotalMB,
			"ramPct":    ramPct,
			"battery":   bat.Capacity,
			"batStatus": bat.Status,
			"netIface":  net.Interface,
			"rxBytes":   net.RXBytes,
			"txBytes":   net.TXBytes,
			"uptime":    up.Seconds,
		}
	})

	// pingDevice: checks whether the device is responding (for status polling)
	w.Bind("pingDevice", func() bool {
		uiState.mu.Lock()
		dev := uiState.dev
		uiState.mu.Unlock()
		if dev == nil {
			return false
		}
		_, err := dev.Handshake()
		return err == nil
	})

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

	w.Bind("wake", func() { withDev(func(d *core.Device) { d.Wake() }) })
	w.Bind("sleep", func() { withDev(func(d *core.Device) { d.Sleep() }) })
	w.Bind("reboot", func() {
		withDev(func(d *core.Device) { d.Reboot() })
	})

	w.Bind("showPage", func(page int) {
		withDev(func(d *core.Device) {
			d.WriteNumRegisters([]core.NumRegister{
				{RegID: core.RegCurrentPage, Value: uint32(page)},
			})
		})
	})

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

	// convertImageToACF: converts an image (base64) to RGB565 raw for the mini screen.
	// The display is 240×240. Returns the bytes as []int for JS.
	w.Bind("convertImageToACF", func(b64data string, targetW int, targetH int) map[string]interface{} {
		if targetW <= 0 {
			targetW = 240
		}
		if targetH <= 0 {
			targetH = 240
		}

		raw, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return map[string]interface{}{"error": "base64 decode: " + err.Error()}
		}

		img, format, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return map[string]interface{}{"error": "image decode: " + err.Error()}
		}
		log.Printf("convertImageToACF: format=%s bounds=%v → %dx%d", format, img.Bounds(), targetW, targetH)

		bounds := img.Bounds()
		srcW := bounds.Max.X - bounds.Min.X
		srcH := bounds.Max.Y - bounds.Min.Y

		// Scale with nearest-neighbor if necessary.
		var rgba *image.RGBA
		if srcW != targetW || srcH != targetH {
			dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
			for y := 0; y < targetH; y++ {
				for x := 0; x < targetW; x++ {
					srcX := bounds.Min.X + x*srcW/targetW
					srcY := bounds.Min.Y + y*srcH/targetH
					dst.Set(x, y, img.At(srcX, srcY))
				}
			}
			rgba = dst
		} else {
			rgba = image.NewRGBA(bounds)
			draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)
		}

		// Convert to RGB565 big-endian: RRRRR GGGGGG BBBBB
		pixelCount := targetW * targetH
		acf := make([]byte, pixelCount*2)
		for y := 0; y < targetH; y++ {
			for x := 0; x < targetW; x++ {
				c := rgba.RGBAAt(x, y)
				r5 := uint16(c.R) >> 3
				g6 := uint16(c.G) >> 2
				b5 := uint16(c.B) >> 3
				rgb565 := (r5 << 11) | (g6 << 5) | b5
				idx := (y*targetW + x) * 2
				acf[idx] = byte(rgb565 >> 8)
				acf[idx+1] = byte(rgb565)
			}
		}

		result := make([]int, len(acf))
		for i, b := range acf {
			result[i] = int(b)
		}

		return map[string]interface{}{
			"data":   result,
			"width":  targetW,
			"height": targetH,
			"size":   len(acf),
		}
	})

	w.Bind("listPorts", func() []string {
		ports, err := core.FindSerialDevices()
		if err != nil {
			return []string{}
		}
		return ports
	})

	w.Bind("setImage", func(payload string) {
		var p struct {
			Asset string `json:"asset"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			log.Println("setImage: parse:", err)
		}
		log.Println("setImage:", p.Asset)
	})

	onViewChange = func() {
		w.Dispatch(func() {
			w.Eval(`window.onViewChanged && window.onViewChanged()`)
		})
	}

	w.Navigate("http://127.0.0.1:8481")
	w.Run()
	return nil
}

// ── runUIMetricsLoop ──────────────────────────────────────────

// runUIMetricsLoop syncs metrics using the already-connected device.
// If the device fails, it clears the state and notifies the UI.
func runUIMetricsLoop(ctx context.Context, dev *core.Device, w wv.WebView) {
	defer func() {
		w.Dispatch(func() {
			w.Eval(`window.onDaemonStopped && window.onDaemonStopped()`)
		})
	}()

	if err := syncDateTime(dev); err != nil {
		log.Println("runUIMetricsLoop: syncDateTime:", err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := syncCycle(dev, &DaemonConfig{Interval: 5 * time.Second, Log: io.Discard}); err != nil {
				log.Println("runUIMetricsLoop: syncCycle error — device disconnected:", err)
				uiState.mu.Lock()
				uiState.dev = nil
				if uiState.daemonCancel != nil {
					uiState.daemonCancel()
					uiState.daemonCancel = nil
				}
				uiState.mu.Unlock()
				return
			}
		}
	}
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
