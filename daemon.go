package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/FreyreCorona/SideCar/core"
	"github.com/FreyreCorona/SideCar/metrics"
)

// DaemonConfig configura el comportamiento del daemon de sincronización.
type DaemonConfig struct {
	// Device es la ruta del puerto serial o "auto".
	Device string
	// Baud es la velocidad de comunicación.
	Baud int
	// Interval es el tiempo entre cada ciclo de actualización.
	Interval time.Duration
	// Log recibe los mensajes de estado. Puede ser nil.
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

// RunDaemon conecta al dispositivo y envía métricas continuamente hasta que
// el contexto sea cancelado o el dispositivo se desconecte.
func RunDaemon(ctx context.Context, cfg DaemonConfig) error {
	cfg.log("→ daemon iniciando (intervalo: %s)", cfg.interval())

	dev, err := connect(cfg.Device, cfg.Baud)
	if err != nil {
		return fmt.Errorf("daemon: conexión fallida: %w", err)
	}
	defer dev.Close()

	if cfg.Log != nil {
		dev.SetLogger(cfg.Log)
	}

	cfg.log("✓ dispositivo conectado")

	// Enviar fecha/hora inicial antes de entrar al loop
	if err := syncDateTime(dev); err != nil {
		cfg.log("  advertencia: no se pudo sincronizar fecha/hora: %v", err)
	}

	ticker := time.NewTicker(cfg.interval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cfg.log("→ daemon detenido")
			return nil

		case <-ticker.C:
			if err := syncCycle(dev, &cfg); err != nil {
				return fmt.Errorf("daemon: error en ciclo de sync: %w", err)
			}
		}
	}
}

// syncCycle ejecuta un ciclo completo: recolecta métricas y las envía al dispositivo.
func syncCycle(dev *core.Device, cfg *DaemonConfig) error {
	// ── Métricas numéricas ────────────────────────────────────────────────────
	cpu := metrics.CollectCPUMetrics()
	mem := metrics.CollectMemoryMetrics()
	bat := metrics.CollectBatteryMetrics()
	net := metrics.CollectNetworkMetrics()

	numRegs := []core.NumRegister{
		// CPU
		{RegID: core.RegCPUUsage, Value: uint32(cpu.UsagePercent)},

		// Memoria: enviamos usado y total como porcentaje de uso
		{RegID: core.RegGPUUsage, Value: memUsagePct(mem)},

		// Batería
		{RegID: core.RegBatteryPercent, Value: uint32(bat.Capacity)},
		{RegID: core.RegBatteryType, Value: core.BatteryLevel(bat.Capacity)},

		// Red: RX/TX en KB (los registros son uint32, suficiente para la mayoría de casos)
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
	// Interfaz de red activa
	if net.Interface != "" {
		if _, err := dev.WriteStringRegister(core.RegWifiSSID, []byte(net.Interface)); err != nil {
			cfg.log("  advertencia: WriteStringRegister interface: %v", err)
		}
	}

	// Estado de batería
	if bat.Status != "" {
		if _, err := dev.WriteStringRegister(core.RegBTName, []byte(bat.Status)); err != nil {
			cfg.log("  advertencia: WriteStringRegister battery status: %v", err)
		}
	}

	// ── Fecha y hora ──────────────────────────────────────────────────────────
	if err := syncDateTime(dev); err != nil {
		cfg.log("  advertencia: syncDateTime: %v", err)
	}

	return nil
}

// syncDateTime envía la fecha y hora actuales al dispositivo.
// Formato: fecha = 0xYYYYMMDD, hora = 0xHHMMSS
func syncDateTime(dev *core.Device) error {
	now := time.Now()

	// Construir el valor numérico hex-encoded igual que el JS:
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

// memUsagePct convierte métricas de memoria a porcentaje (0-100).
func memUsagePct(m metrics.MemoryMetrics) uint32 {
	if m.TotalMB == 0 {
		return 0
	}
	return uint32(100 * m.UsedMB / m.TotalMB)
}
