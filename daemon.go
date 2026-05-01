package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/FreyreCorona/SideCar/core"
	"github.com/FreyreCorona/SideCar/metrics"
)

// DaemonConfig holds the configuration for the metrics sync daemon.
type DaemonConfig struct {
	// Device is the serial port path or "auto".
	Device string
	// Baud is the communication baud rate.
	Baud int
	// Interval is the time between each sync cycle.
	Interval time.Duration
	// Log receives status messages. Can be nil.
	Log io.Writer
}

func (c *DaemonConfig) interval() time.Duration {
	if c.Interval > 0 {
		return c.Interval
	}
	return 5 * time.Second
}

func (c *DaemonConfig) log(format string, args ...any) {
	if c.Log != nil {
		fmt.Fprintf(c.Log, format+"\n", args...)
	}
}

// RunDaemon connects to the device and continuously sends metrics until
// the context is cancelled or the device disconnects.
func RunDaemon(ctx context.Context, cfg DaemonConfig) error {
	cfg.log("→ daemon starting (interval: %s)", cfg.interval())

	dev, err := connect(cfg.Device, cfg.Baud)
	if err != nil {
		return fmt.Errorf("daemon: connection failed: %w", err)
	}
	if cfg.Log != nil {
		dev.SetLogger(cfg.Log)
	}

	cfg.log("✓ device connected")

	// Send initial date/time before entering the loop
	if err := syncDateTime(dev); err != nil {
		cfg.log("  warning: could not sync date/time: %v", err)
	}

	ticker := time.NewTicker(cfg.interval())
	defer ticker.Stop()

	// Hot-plug: monitor device path existence
	devicePath := getDevicePath(dev)
	hotPlugTicker := time.NewTicker(2 * time.Second)
	defer hotPlugTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			cfg.log("→ daemon stopped")
			if dev != nil {
				dev.Close()
			}
			return nil

		case <-ticker.C:
			if err := syncCycle(dev, &cfg); err != nil {
				cfg.log("  error: sync cycle failed: %v", err)
				if dev != nil {
					dev.Close()
					dev = nil
				}
				newDev := reconnectLoop(ctx, cfg)
				if newDev == nil {
					cfg.log("  failed to reconnect, daemon exiting")
					return fmt.Errorf("daemon: reconnection failed")
				}
				dev = newDev
				devicePath = getDevicePath(dev)
				cfg.log("✓ reconnected to device")
				if err := syncDateTime(dev); err != nil {
					cfg.log("  warning: could not sync date/time after reconnect: %v", err)
				}
			}

		case <-hotPlugTicker.C:
			// Check if device was physically disconnected
			if dev != nil && devicePath != "" && !deviceExists(devicePath) {
				cfg.log("  device physically disconnected: %s", devicePath)
				dev.Close()
				dev = nil
				newDev := reconnectLoop(ctx, cfg)
				if newDev == nil {
					cfg.log("  failed to reconnect, daemon exiting")
					return fmt.Errorf("daemon: reconnection failed")
				}
				dev = newDev
				devicePath = getDevicePath(dev)
				cfg.log("✓ reconnected to device after hot-plug")
				if err := syncDateTime(dev); err != nil {
					cfg.log("  warning: could not sync date/time after reconnect: %v", err)
				}
			}
		}
	}
}

// getDevicePath attempts to find the device path by checking current ports.
// This is a best-effort approach since Device doesn't expose the path directly.
func getDevicePath(dev *core.Device) string {
	// Try handshake to see if device is still responsive
	// If it fails, the device might be physically disconnected
	_, err := dev.Handshake()
	if err == nil {
		// Device is responsive, try to find it in the port list
		ports, err := core.FindSerialDevices()
		if err == nil && len(ports) > 0 {
			// Return the first available port (simplified)
			return ports[0]
		}
	}
	return ""
}

// deviceExists checks if a device path still exists in /dev
func deviceExists(path string) bool {
	if path == "" {
		// If we don't have a path, try to find any available port
		ports, err := core.FindSerialDevices()
		return err == nil && len(ports) > 0
	}
	// Check if the specific path exists
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// reconnectLoop attempts to reconnect to the device until successful or context is cancelled.
func reconnectLoop(ctx context.Context, cfg DaemonConfig) *core.Device {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
			cfg.log("  attempting to reconnect...")
			dev, err := connect(cfg.Device, cfg.Baud)
			if err == nil {
				if cfg.Log != nil {
					dev.SetLogger(cfg.Log)
				}
				return dev
			}
			cfg.log("  reconnect failed: %v", err)
		}
	}
}

// syncCycle runs a full cycle: collects metrics and sends them to the device.
func syncCycle(dev *core.Device, cfg *DaemonConfig) error {
	cpu := metrics.CollectCPUMetrics()
	mem := metrics.CollectMemoryMetrics()
	bat := metrics.CollectBatteryMetrics()
	net := metrics.CollectNetworkMetrics()

	numRegs := []core.NumRegister{
		// CPU
		{RegID: core.RegCPUUsage, Value: uint32(cpu.UsagePercent)},

		// Memory: send used/total as usage percentage
		{RegID: core.RegGPUUsage, Value: memUsagePct(mem)},

		// Battery
		{RegID: core.RegBatteryPercent, Value: uint32(bat.Capacity)},
		{RegID: core.RegBatteryType, Value: core.BatteryLevel(bat.Capacity)},

		// WiFi Quality
		{RegID: core.RegWifiQuality, Value: core.WifiQualityLevel(net.WifiQuality)},

		// Network Traffic: RX/TX counters
		{RegID: core.RegMediaDuration, Value: uint32(net.RXBytes / 1024)},
		{RegID: core.RegMediaNow, Value: uint32(net.TXBytes / 1024)},
	}

	if _, err := dev.WriteNumRegisters(numRegs); err != nil {
		return fmt.Errorf("WriteNumRegisters: %w", err)
	}

	cfg.log("  CPU: %.1f%%  RAM: %d/%dMB  Bat: %d%%  RX: %dKB TX: %dKB WiFi: %s (%d%%)",
		cpu.UsagePercent,
		mem.UsedMB, mem.TotalMB,
		bat.Capacity,
		net.RXBytes/1024, net.TXBytes/1024,
		net.WifiSSID, net.WifiQuality,
	)

	// WiFi SSID
	if net.WifiSSID != "" {
		if _, err := dev.WriteStringRegister(core.RegWifiSSID, []byte(net.WifiSSID)); err != nil {
			cfg.log("  warning: WriteStringRegister SSID: %v", err)
		}
	} else if net.Interface != "" {
		// Fallback to interface name if no SSID
		if _, err := dev.WriteStringRegister(core.RegWifiSSID, []byte(net.Interface)); err != nil {
			cfg.log("  warning: WriteStringRegister interface: %v", err)
		}
	}

	// Battery status
	if bat.Status != "" {
		if _, err := dev.WriteStringRegister(core.RegBTName, []byte(bat.Status)); err != nil {
			cfg.log("  warning: WriteStringRegister battery status: %v", err)
		}
	}

	// Date and time
	if err := syncDateTime(dev); err != nil {
		cfg.log("  warning: syncDateTime: %v", err)
	}

	return nil
}

// syncDateTime sends the current date and time to the device.
// So year=2025, month=03, day=07 → "20250307" parsed as hex → 0x20250307.
// Each byte is BCD: the device reads 0x20, 0x25, 0x03, 0x07 independently.
func syncDateTime(dev *core.Device) error {
	t := time.Now()
	toBCD := func(v int) uint32 {
		return uint32((v/10)<<4 | (v % 10))
	}
	y := t.Year()

	dateVal :=
		toBCD(y/100)<<24 |
			toBCD(y%100)<<16 |
			toBCD(int(t.Month()))<<8 |
			toBCD(t.Day())

	timeVal :=
		toBCD(t.Hour())<<16 |
			toBCD(t.Minute())<<8 |
			toBCD(t.Second())

	_, err := dev.WriteNumRegisters([]core.NumRegister{
		{RegID: core.RegDate, Value: dateVal},
		{RegID: core.RegTime, Value: timeVal},
	})

	if err != nil {
		return fmt.Errorf("syncDateTime: write registers: %w", err)
	}

	return nil
}

// memUsagePct converts memory metrics to a percentage (0-100).
func memUsagePct(m metrics.MemoryMetrics) uint32 {
	if m.TotalMB == 0 {
		return 0
	}
	return uint32(100 * m.UsedMB / m.TotalMB)
}
