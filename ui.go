package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/FreyreCorona/SideCar/core"
	"github.com/FreyreCorona/SideCar/metrics"
	"github.com/FreyreCorona/SideCar/power"

	wv "github.com/abemedia/go-webview"
	_ "github.com/abemedia/go-webview/embedded"
)

//go:embed ui/*
var uiFS embed.FS

type UIState struct {
	dev          atomic.Pointer[core.Device]
	daemonCtx    context.Context
	daemonCancel context.CancelFunc
	win          wv.WebView
	uploadProg   func(int)
	powerMonitor power.Monitor
	uploading    atomic.Bool
}

var uiState UIState

func runUI(logOut io.Writer) error {
	debug := os.Getenv("SIDECAR_DEBUG") == "1"
	w := wv.New(debug)
	defer w.Destroy()

	uiState.win = w
	uiState.uploadProg = func(pct int) {
		w.Dispatch(func() {
			w.Eval(fmt.Sprintf(`window.onUploadProgress && window.onUploadProgress(%d)`, pct))
		})
	}

	// Setup Power Monitor
	uiState.powerMonitor = power.NewMonitor()
	uiState.powerMonitor.Start(func(event power.PowerEvent) {
		dev := uiState.dev.Load()
		if dev == nil {
			return
		}
		if event == power.Sleep {
			log.Println("UI: System sleep detected, sending device to sleep")
			dev.Sleep()
		} else if event == power.Wake {
			log.Println("UI: System wake detected, waking device")
			dev.Wake()
		}
	})
	defer uiState.powerMonitor.Stop()

	w.SetTitle("SideCar")
	w.SetSize(960, 620, wv.HintNone)

	// HTTP server
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))

	server := &http.Server{Addr: "127.0.0.1:8481", Handler: mux}
	go func() {
		log.Println("UI server: http://127.0.0.1:8481")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	//Go → JS bindings

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
	w.Bind("startDaemon", func() bool {
		if uiState.daemonCancel != nil {
			return true
		}

		dev, err := core.AutoConnect(115200)
		if err != nil {
			log.Println("startDaemon: connect:", err)
			return false
		}
		uiState.dev.Store(dev)

		ctx, cancel := context.WithCancel(context.Background())
		uiState.daemonCtx = ctx
		uiState.daemonCancel = cancel

		go runUIMetricsLoop(ctx, dev, w, logOut)

		return true
	})
	w.Bind("stopDaemon", func() {
		if uiState.daemonCancel != nil {
			uiState.daemonCancel()
			uiState.daemonCancel = nil
			uiState.daemonCtx = nil
		}
		if dev := uiState.dev.Load(); dev != nil {
			dev.Close()
			uiState.dev.Store(nil)
		}
	})
	w.Bind("getStats", func() map[string]any {
		cpu := metrics.CollectCPUMetrics()
		mem := metrics.CollectMemoryMetrics()
		bat := metrics.CollectBatteryMetrics()
		net := metrics.CollectNetworkMetrics()
		up := metrics.CollectUptimeMetrics()

		ramPct := 0
		if mem.TotalMB > 0 {
			ramPct = 100 * mem.UsedMB / mem.TotalMB
		}

		return map[string]any{
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
	w.Bind("pingDevice", func() bool {
		dev := uiState.dev.Load()
		if dev == nil {
			return false
		}
		_, err := dev.Handshake()
		return err == nil
	})
	w.Bind("setBrightness", func(level int) {
		dev := uiState.dev.Load()
		if dev == nil {
			return
		}
		if level < 0 {
			level = 0
		}
		if level > 100 {
			level = 100
		}
		if err := dev.SetBrightness(uint8(level)); err != nil {
			log.Println("setBrightness:", err)
		}
	})
	w.Bind("wake", func() {
		if dev := uiState.dev.Load(); dev != nil {
			dev.Wake()
		}
	})
	w.Bind("sleep", func() {
		if dev := uiState.dev.Load(); dev != nil {
			dev.Sleep()
		}
	})
	w.Bind("reboot", func() {
		if dev := uiState.dev.Load(); dev != nil {
			dev.Reboot()
		}
	})
	w.Bind("showPage", func(page int) {
		if dev := uiState.dev.Load(); dev != nil {
			dev.WriteNumRegisters([]core.NumRegister{
				{RegID: core.RegCurrentPage, Value: uint32(page)},
			})
		}
	})
	w.Bind("uploadFile", func(bytesArr []int, fileType string) map[string]any {
		dev := uiState.dev.Load()

		if dev == nil {
			return map[string]any{"error": "device not connected"}
		}

		data := make([]byte, len(bytesArr))
		for i, b := range bytesArr {
			data[i] = byte(b)
		}

		// When target is texture or texture_gif, the device expects a full ACF
		// project file (header + resource blocks + project data), not raw RGB565.
		// Converted images arrive wrapped with an 8-byte header
		// (size LE + width LE + height LE); strip it and wrap the raw RGB565
		// in the ACF project format. The display is always 240×240.
		ft := core.FileType(fileType)
		if ft == core.FileTypeTexture || ft == core.FileTypeTextureGIF {
			if rgb, w, h, err := core.UnwrapRGB565(data); err == nil {
				if w != core.DisplaySize || h != core.DisplaySize {
					return map[string]any{"error": fmt.Sprintf("texture image must be %dx%d, got %dx%d", core.DisplaySize, core.DisplaySize, w, h)}
				}
				project, err := core.BuildACF(rgb)
				if err != nil {
					log.Printf("uploadFile: BuildACF: %v", err)
					return map[string]any{"error": "ACF project build failed: " + err.Error()}
				}
				log.Printf("uploadFile: wrapped %d bytes of image data in ACF project (%d bytes)", len(rgb), len(project))
				data = project
			}
		}

		// ~150ms per KB at 115200 baud with protocol overhead, minimum 2 min
		est := max(time.Duration(len(data)/1024)*150*time.Millisecond, 2*time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), est)
		defer cancel()

		uiState.uploading.Store(true)
		defer uiState.uploading.Store(false)

		cfg := core.UploadConfig{
			OnProgress: func(p core.UploadProgress) {
				if uiState.uploadProg != nil {
					uiState.uploadProg(p.Percent)
				}
			},
		}

		if err := dev.UploadFile(ctx, data, ft, cfg); err != nil {
			return map[string]any{"error": err.Error()}
		}

		// Device reboots after DownloadComplete — do NOT try to Handshake.
		// The old post-upload Handshake always failed with "Port has been closed".
		// The daemon will detect the reconnect and re-establish the session.

		return map[string]any{"ok": true}
	})

	// convertImageToACF: converts an image (base64) to RGB565 ACF for the mini screen.
	// Supports PNG, JPEG, and GIF (including animated). The display is always
	// 240×240; any source size is bilinear-scaled. targetW/targetH are kept for
	// backward compatibility with the JS binding but are ignored.
	w.Bind("convertImageToACF", func(b64data string, _, _ int) map[string]any {
		raw, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			return map[string]any{"error": "base64 decode: " + err.Error()}
		}

		// First, use the generic decoder (handles PNG, JPEG, static GIF).
		img, format, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			prefix := raw
			if len(prefix) > 32 {
				prefix = prefix[:32]
			}
			log.Printf("convertImageToACF: decode error, len=%d head=%x", len(raw), prefix)
			return map[string]any{"error": "image decode: " + err.Error()}
		}
		log.Printf("convertImageToACF: format=%s bounds=%v → %dx%d", format, img.Bounds(), core.DisplaySize, core.DisplaySize)

		// If it's a GIF, reload with DecodeAll for animation support.
		if format == "gif" {
			if gifData, gifErr := gif.DecodeAll(bytes.NewReader(raw)); gifErr == nil && len(gifData.Image) > 1 {
				return convertGIFToACF(gifData)
			}
		}

		rgb, err := core.ImageToRGB565(img, core.DisplaySize, core.DisplaySize)
		if err != nil {
			return map[string]any{"error": "convert: " + err.Error()}
		}
		acf, err := core.WrapRGB565(rgb, core.DisplaySize, core.DisplaySize)
		if err != nil {
			return map[string]any{"error": "convert: " + err.Error()}
		}
		result := make([]int, len(acf))
		for i, b := range acf {
			result[i] = int(b)
		}

		return map[string]any{
			"data":   result,
			"width":  core.DisplaySize,
			"height": core.DisplaySize,
			"size":   len(acf),
			"frames": 1,
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

// runUIMetricsLoop syncs metrics using the already-connected device.
// If the device fails, it clears the state and notifies the UI.
func runUIMetricsLoop(ctx context.Context, dev *core.Device, w wv.WebView, logOut io.Writer) {
	defer func() {
		w.Dispatch(func() {
			w.Eval(`window.onDaemonStopped && window.onDaemonStopped()`)
		})
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	firstSync := true

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if uiState.uploading.Load() {
				continue
			}

			if firstSync {
				firstSync = false
				if err := syncDateTime(dev); err != nil {
					log.Println("runUIMetricsLoop: syncDateTime:", err)
				}
			}

			if err := syncCycle(dev, &DaemonConfig{Interval: 5 * time.Second, Log: logOut}); err != nil {
				if uiState.uploading.Load() {
					continue
				}

				log.Println("runUIMetricsLoop: syncCycle error — device disconnected:", err)

				uiState.dev.Store(nil)

				if uiState.daemonCancel != nil {
					uiState.daemonCancel()
					uiState.daemonCancel = nil
				}

				dev = nil
				return
			}
		}
	}
}

// ── Image conversion helpers ─────────────────────────────────────────────

func convertGIFToACF(g *gif.GIF) map[string]any {
	frameCount := len(g.Image)
	log.Printf("convertGIFToACF: %d frames, loop=%d, bounds=%v → %dx%d", frameCount, g.LoopCount, g.Config, core.DisplaySize, core.DisplaySize)

	bgColor := color.RGBA{0, 0, 0, 255}
	if pal, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(pal) {
		if c := color.RGBAModel.Convert(pal[g.BackgroundIndex]); c != nil {
			bgColor = c.(color.RGBA)
		}
	}

	canvas := image.NewRGBA(image.Rect(0, 0, core.DisplaySize, core.DisplaySize))
	fillRGBA(canvas, bgColor)

	var acfFrames [][]byte
	frameDelays := make([]int, frameCount)

	for i, src := range g.Image {
		bounds := src.Bounds()

		draw.Draw(canvas, bounds, src, bounds.Min, draw.Over)

		rgb, err := core.ImageToRGB565(canvas, core.DisplaySize, core.DisplaySize)
		if err != nil {
			return map[string]any{"error": "convert frame: " + err.Error()}
		}
		wrapped, err := core.WrapRGB565(rgb, core.DisplaySize, core.DisplaySize)
		if err != nil {
			return map[string]any{"error": "convert frame: " + err.Error()}
		}
		acfFrames = append(acfFrames, wrapped)
		frameDelays[i] = g.Delay[i]

		if len(g.Disposal) > i {
			switch g.Disposal[i] {
			case gif.DisposalBackground, gif.DisposalPrevious:
				fillRect(canvas, bounds, bgColor)
			}
		}
	}

	frames := make([]map[string]any, len(acfFrames))
	var totalSize int
	for i, f := range acfFrames {
		conv := make([]int, len(f))
		for j, b := range f {
			conv[j] = int(b)
		}
		frames[i] = map[string]any{
			"data":  conv,
			"size":  len(f),
			"delay": g.Delay[i],
		}
		totalSize += len(f)
	}

	return map[string]any{
		"frames":      frames,
		"width":       core.DisplaySize,
		"height":      core.DisplaySize,
		"size":        totalSize,
		"frameCount":  frameCount,
		"frameDelays": frameDelays,
		"loopCount":   g.LoopCount,
	}
}

func fillRGBA(img *image.RGBA, c color.Color) {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, c color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y && y < img.Bounds().Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X && x < img.Bounds().Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}
