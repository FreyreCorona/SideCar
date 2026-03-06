package main

import (
	"context"
	"fmt"
	"io"
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
	defer dev.Close()

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

	for {
		select {
		case <-ctx.Done():
			cfg.log("→ daemon stopped")
			return nil

		case <-ticker.C:
			if err := syncCycle(dev, &cfg); err != nil {
				return fmt.Errorf("daemon: sync cycle error: %w", err)
			}
		}
	}
}

// syncCycle runs a full cycle: collects metrics and sends them to the device.
func syncCycle(dev *core.Device, cfg *DaemonConfig) error {
	// ── Numeric metrics ───────────────────────────────────────────────────────
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

		// Network: RX/TX in KB (registers are uint32, sufficient for most cases)
		{RegID: core.RegWifiQuality, Value: uint32(net.RXBytes / 1024)},
		{RegID: core.RegWifiStatus, Value: uint32(net.TXBytes / 1024)},
	}

	if _, err := dev.WriteNumRegisters(numRegs); err != nil {
		return fmt.Errorf("WriteNumRegisters: %w", err)
	}

	cfg.log("  CPU: %.1f%%  RAM: %d/%dMB  Bat: %d%%  RX: %dKB TX: %dKB",
		cpu.UsagePercent,
		mem.UsedMB, mem.TotalMB,
		bat.Capacity,
		net.RXBytes/1024, net.TXBytes/1024,
	)

	// ── Strings ───────────────────────────────────────────────────────────────
	// Active network interface
	if net.Interface != "" {
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

	// ── Date and time ──────────────────────────────────────────────────────────
	if err := syncDateTime(dev); err != nil {
		cfg.log("  warning: syncDateTime: %v", err)
	}

	return nil
}

// syncDateTime sends the current date and time to the device.
// Format: date = 0xYYYYMMDD, time = 0xHHMMSS
func syncDateTime(dev *core.Device) error {
	now := time.Now()

	// Build the hex-encoded numeric value matching the JS logic:
	// dateStr = `0x${year}${month}${day}` → Number(dateStr)
	dateVal := uint32(now.Year())*0x10000 +
		uint32(now.Month())*0x100 +
		uint32(now.Day())

	timeVal := uint32(now.Hour())*0x10000 +
		uint32(now.Minute())*0x100 +
		uint32(now.Second())

	_, err := dev.WriteNumRegisters([]core.NumRegister{
		{RegID: core.RegDate, Value: dateVal},
		{RegID: core.RegTime, Value: timeVal},
	})
	return err
}

// memUsagePct converts memory metrics to a percentage (0-100).
func memUsagePct(m metrics.MemoryMetrics) uint32 {
	if m.TotalMB == 0 {
		return 0
	}
	return uint32(100 * m.UsedMB / m.TotalMB)
}
