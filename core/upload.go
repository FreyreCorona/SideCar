package core

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// Tipos de archivo y direcciones de memoria
// ─────────────────────────────────────────────

// FileType identifica el tipo de binario a transferir.
type FileType string

const (
	FileTypeCPU1         FileType = "cpu1"
	FileTypeCPU0Config   FileType = "cpu0_config"
	FileTypeCalibration  FileType = "calibration"
	FileTypeEraseStore   FileType = "eraseStoreSpace"
	FileTypeHWConfig     FileType = "hwconfig"
	FileTypeProduct      FileType = "product"
	FileTypeTexture      FileType = "texture"
	FileTypeTextureGIF   FileType = "texture_gif"
	FileTypeUpgCfg       FileType = "upg_cfg"
	FileTypeBootloader   FileType = "bootloader"
)

// fileTypeAddrs mapea cada FileType a su dirección de memoria en el MCU.
var fileTypeAddrs = map[FileType]uint32{
	FileTypeCPU1:       0x08040000,
	FileTypeCPU0Config: 0x080F8000,
	FileTypeCalibration:0x080FB000,
	FileTypeEraseStore: 0x080FC000,
	FileTypeHWConfig:   0x080FE008,
	FileTypeProduct:    0x080FF008,
	FileTypeTexture:    0x08100000,
	FileTypeTextureGIF: 0x08500000,
	FileTypeUpgCfg:     0x080F7000,
	FileTypeBootloader: 0x08000000,
}

const maxUploadFileSize = 6436 * 1024 // 6436 KB

// Estados del dispositivo reportados por GetDownloadStatus
const (
	downloadStatePrep     = 0x10 // preparación de descarga
	downloadStateActive   = 0x11 // descarga en curso (resume posible)
	downloadStateAHMI     = 0x20 // modo AHMI (ya tiene un archivo)
)

// Código de respuesta "procesando" — el dispositivo enviará una respuesta final después
const respProcessing = uint32(0xFFFFFFFF)

// ─────────────────────────────────────────────
// Configuración y progreso
// ─────────────────────────────────────────────

// UploadProgress representa el estado actual de la transferencia.
type UploadProgress struct {
	BytesSent  int64
	TotalBytes int64
	Percent    int // 0–100
}

// UploadConfig permite ajustar timeouts y callbacks de progreso.
type UploadConfig struct {
	// ChunkTimeout es el timeout por chunk de datos. Por defecto: 5s.
	ChunkTimeout time.Duration
	// OnProgress se llama cada vez que el porcentaje avanza ≥5 puntos.
	// Es seguro dejarlo nil.
	OnProgress func(UploadProgress)
}

func (c *UploadConfig) chunkTimeout() time.Duration {
	if c.ChunkTimeout > 0 {
		return c.ChunkTimeout
	}
	return 5 * time.Second
}

// ─────────────────────────────────────────────
// UploadFile
// ─────────────────────────────────────────────

// UploadFile transfiere data al MCU en la dirección correspondiente a fileType.
// El protocolo implementado es el mismo que upload-file en electron.js.
func (d *Device) UploadFile(data []byte, fileType FileType, cfg UploadConfig) error {
	addr, ok := fileTypeAddrs[fileType]
	if !ok {
		return fmt.Errorf("uploadFile: tipo de archivo desconocido %q", fileType)
	}

	if len(data) > maxUploadFileSize {
		return fmt.Errorf("uploadFile: archivo demasiado grande (%d bytes, máximo %d)", len(data), maxUploadFileSize)
	}

	fileSize := uint32(len(data))
	fileID := md5.Sum(data)

	fmt.Fprintf(d.log, "→ UploadFile type=%s addr=0x%08X size=%d\n", fileType, addr, fileSize)

	// ── 1. Handshake ──────────────────────────────────────────────────────────
	if _, err := d.Handshake(); err != nil {
		return fmt.Errorf("uploadFile: handshake: %w", err)
	}

	// ── 2. GetDownloadStatus ──────────────────────────────────────────────────
	status, err := d.GetDownloadStatus()
	if err != nil {
		return fmt.Errorf("uploadFile: getDownloadStatus: %w", err)
	}

	startOffset := uint32(0)

	switch status.Status {
	case downloadStatePrep, downloadStateActive:
		// Si el archivo es el mismo podemos reanudar desde el offset guardado
		if status.FileID == fileID {
			startOffset = status.Offset
			fmt.Fprintf(d.log, "  resuming from offset %d\n", startOffset)
		}
		// Si es diferente, empezamos desde 0 (startOffset ya es 0)

	case downloadStateAHMI:
		if status.FileID == fileID {
			// Ya está grabado — nada que hacer
			fmt.Fprintln(d.log, "  archivo ya presente en el dispositivo")
			d.reportProgress(cfg.OnProgress, int64(fileSize), int64(fileSize))
			return nil
		}
		// Hay otro archivo: necesitamos cambiar el estado antes de grabar
		if err := d.switchToDownloadMode(); err != nil {
			return fmt.Errorf("uploadFile: switchState: %w", err)
		}

	default:
		return fmt.Errorf("uploadFile: estado de descarga inválido: 0x%02X", status.Status)
	}

	// ── 3. RequestDownload ────────────────────────────────────────────────────
	dlResult, err := d.RequestDownload(addr, fileSize, fileID)
	if err != nil {
		return fmt.Errorf("uploadFile: requestDownload: %w", err)
	}

	maxPageSize := dlResult.MaxPageSize
	if maxPageSize == 0 {
		return fmt.Errorf("uploadFile: dispositivo devolvió maxPageSize=0")
	}

	// Respuesta 0xFFFFFFFF → el dispositivo sigue preparándose; esperamos la final
	if dlResult.Response == respProcessing {
		fmt.Fprintln(d.log, "  requestDownload: esperando respuesta final del dispositivo")
		if err := d.waitForDownloadReady(); err != nil {
			return fmt.Errorf("uploadFile: requestDownload processing: %w", err)
		}
	} else if dlResult.Response != 0 {
		return fmt.Errorf("uploadFile: requestDownload rechazado: 0x%08X", dlResult.Response)
	}

	// ── 4. Transferencia de datos ─────────────────────────────────────────────
	offset := startOffset
	lastReportedPct := -1

	for offset < fileSize {
		end := offset + maxPageSize
		if end > fileSize {
			end = fileSize
		}
		chunk := data[offset:end]

		if err := d.sendChunk(offset, chunk, cfg.chunkTimeout()); err != nil {
			return fmt.Errorf("uploadFile: sendChunk offset=%d: %w", offset, err)
		}

		offset = end

		// Progreso: notificar solo cuando cambia ≥5%
		pct := int(100 * int64(offset) / int64(fileSize))
		if cfg.OnProgress != nil && pct/5 > lastReportedPct/5 {
			lastReportedPct = pct
			cfg.OnProgress(UploadProgress{
				BytesSent:  int64(offset),
				TotalBytes: int64(fileSize),
				Percent:    (pct / 5) * 5,
			})
		}
	}

	d.reportProgress(cfg.OnProgress, int64(fileSize), int64(fileSize))

	// ── 5. DownloadComplete ───────────────────────────────────────────────────
	// Ignoramos errores aquí (igual que el JS) porque el dispositivo a veces
	// no responde al complete pero la grabación fue exitosa
	if err := d.DownloadComplete(); err != nil {
		fmt.Fprintf(d.log, "  DownloadComplete: %v (ignorado)\n", err)
	}

	fmt.Fprintln(d.log, "✓ UploadFile completado")
	return nil
}

// ─────────────────────────────────────────────
// Helpers internos del upload
// ─────────────────────────────────────────────

// switchToDownloadMode envía SWITCH_STATE con valor 0x10 para salir del modo AHMI.
func (d *Device) switchToDownloadMode() error {
	fmt.Fprintln(d.log, "→ SwitchState → download mode")

	// El JS envía la cadena hex "10" como content, que es el byte 0x10
	cmd := NewCommand(CmdSwitchState, []byte{0x10})
	resp, err := d.sendWithRetry(cmd, CmdSwitchStateResp, 1*time.Second, 3)
	if err != nil {
		return err
	}

	if len(resp.Content) < 4 {
		return fmt.Errorf("SwitchState response demasiado corta")
	}
	code := binary.BigEndian.Uint32(resp.Content[0:4])
	if code != 0 {
		return fmt.Errorf("SwitchState falló con código 0x%08X", code)
	}
	return nil
}

// waitForDownloadReady espera hasta recibir una respuesta REQUEST_DOWNLOAD_RESPONSE con código 0.
func (d *Device) waitForDownloadReady() error {
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := ReadResponse(d.serial, time.Until(deadline))
		if err != nil {
			return err
		}
		if resp.Type != CmdRequestDownloadResp {
			continue
		}
		if len(resp.Content) < 8 {
			continue
		}
		code := binary.BigEndian.Uint32(resp.Content[4:8])
		if code == 0 {
			return nil
		}
		if code != respProcessing {
			return fmt.Errorf("requestDownload: código de error 0x%08X", code)
		}
	}
}

// sendChunk envía un chunk de datos y maneja el patrón "processing".
func (d *Device) sendChunk(offset uint32, chunk []byte, timeout time.Duration) error {
	content := make([]byte, 4+len(chunk))
	binary.BigEndian.PutUint32(content[0:4], offset)
	copy(content[4:], chunk)

	cmd := NewCommand(CmdDownloadData, content)

	// Reintentamos el chunk si hay timeout
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := d.serial.Write(cmd.Frame()); err != nil {
			return err
		}

		resp, err := ExpectResponse(d.serial, CmdDownloadDataResp, timeout)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				lastErr = err
				continue
			}
			return err
		}

		if len(resp.Content) < 4 {
			lastErr = fmt.Errorf("DownloadData response demasiado corta")
			continue
		}

		code := binary.BigEndian.Uint32(resp.Content[0:4])
		switch code {
		case 0:
			return nil // éxito
		case respProcessing:
			if err := d.waitForChunkDone(timeout); err != nil {
				lastErr = err
				continue
			}
			return nil
		default:
			// Otros códigos → reintentamos el chunk
			lastErr = fmt.Errorf("DownloadData: código 0x%08X", code)
			continue
		}
	}

	return fmt.Errorf("sendChunk: agotados reintentos: %w", lastErr)
}

// waitForChunkDone espera a que el dispositivo confirme que procesó el chunk.
func (d *Device) waitForChunkDone(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := ReadResponse(d.serial, time.Until(deadline))
		if err != nil {
			return err
		}
		if resp.Type != CmdDownloadDataResp {
			continue
		}
		if len(resp.Content) < 4 {
			continue
		}
		code := binary.BigEndian.Uint32(resp.Content[0:4])
		if code == 0 {
			return nil
		}
		if code != respProcessing {
			return fmt.Errorf("DownloadData processing: error 0x%08X", code)
		}
	}
}

func (d *Device) reportProgress(fn func(UploadProgress), sent, total int64) {
	if fn == nil {
		return
	}
	pct := 100
	if total > 0 {
		pct = int(100 * sent / total)
	}
	fn(UploadProgress{BytesSent: sent, TotalBytes: total, Percent: pct})
}

// ─────────────────────────────────────────────
// DownloadComplete (implementación completa con processing)
// ─────────────────────────────────────────────

// DownloadComplete notifica al dispositivo que la transferencia terminó.
// Sobrescribe la versión simple de device.go con soporte para el patrón "processing".
func (d *Device) DownloadComplete() error {
	fmt.Fprintln(d.log, "→ DownloadComplete")

	cmd := NewCommand(CmdDownloadComplete, nil)
	if err := d.serial.Write(cmd.Frame()); err != nil {
		return err
	}

	resp, err := ExpectResponse(d.serial, CmdDownloadCompleteResp, 5*time.Second)
	if err != nil {
		return err
	}

	if len(resp.Content) < 4 {
		return fmt.Errorf("DownloadComplete response demasiado corta")
	}

	code := binary.BigEndian.Uint32(resp.Content[0:4])
	if code == 0 {
		return nil
	}
	if code != respProcessing {
		return fmt.Errorf("DownloadComplete: error 0x%08X", code)
	}

	// Esperamos la respuesta final
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := ReadResponse(d.serial, time.Until(deadline))
		if err != nil {
			return err
		}
		if resp.Type != CmdDownloadCompleteResp {
			continue
		}
		if len(resp.Content) < 4 {
			continue
		}
		code := binary.BigEndian.Uint32(resp.Content[0:4])
		if code == 0 {
			return nil
		}
		if code != respProcessing {
			return fmt.Errorf("DownloadComplete processing: error 0x%08X", code)
		}
	}
}


