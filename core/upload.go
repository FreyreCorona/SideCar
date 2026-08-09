package core

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// File types and memory addresses
// ─────────────────────────────────────────────

// FileType identifies the type of binary to transfer.
type FileType string

const (
	FileTypeCPU1        FileType = "cpu1"
	FileTypeCPU0Config  FileType = "cpu0_config"
	FileTypeCalibration FileType = "calibration"
	FileTypeEraseStore  FileType = "eraseStoreSpace"
	FileTypeHWConfig    FileType = "hwconfig"
	FileTypeProduct     FileType = "product"
	FileTypeTexture     FileType = "texture"
	FileTypeTextureGIF  FileType = "texture_gif"
	FileTypeUpgCfg      FileType = "upg_cfg"
	FileTypeBootloader  FileType = "bootloader"
)

// fileTypeAddrs maps each FileType to its memory address on the MCU.
var fileTypeAddrs = map[FileType]uint32{
	FileTypeCPU1:        0x08040000,
	FileTypeCPU0Config:  0x080F8000,
	FileTypeCalibration: 0x080FB000,
	FileTypeEraseStore:  0x080FC000,
	FileTypeHWConfig:    0x080FE008,
	FileTypeProduct:     0x080FF008,
	FileTypeTexture:     0x08100000,
	FileTypeTextureGIF:  0x08500000,
	FileTypeUpgCfg:      0x080F7000,
	FileTypeBootloader:  0x08000000,
}

const maxUploadFileSize = 6436 * 1024 // 6436 KB

// Device states reported by GetDownloadStatus
const (
	downloadStatePrep   = 0x10 // preparing download
	downloadStateActive = 0x11 // download in progress (resume possible)
	downloadStateAHMI   = 0x20 // AHMI mode (device already has a file)
)

// "processing" response code — the device will send a final response later
const respProcessing = uint32(0xFFFFFFFF)

// ─────────────────────────────────────────────
// Configuration and progress
// ─────────────────────────────────────────────

// UploadProgress represents the current state of the transfer.
type UploadProgress struct {
	BytesSent  int64
	TotalBytes int64
	Percent    int // 0–100
}

// UploadConfig allows adjusting timeouts and progress callbacks.
type UploadConfig struct {
	// ChunkTimeout is the timeout per data chunk. Default: 5s.
	ChunkTimeout time.Duration
	// OnProgress is called whenever the percentage advances by ≥5 points.
	// Safe to leave nil.
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

// UploadFile transfers data to the MCU at the address corresponding to fileType.
// The protocol implemented matches upload-file in electron.js.
// The context allows cancellation — if the caller cancels mid-upload the device
// state is left as-is and the serial port is drained.
func (d *Device) UploadFile(ctx context.Context, data []byte, fileType FileType, cfg UploadConfig) error {
	addr, ok := fileTypeAddrs[fileType]
	if !ok {
		return fmt.Errorf("uploadFile: unknown file type %q", fileType)
	}

	if len(data) > maxUploadFileSize {
		return fmt.Errorf("uploadFile: file too large (%d bytes, maximum %d)", len(data), maxUploadFileSize)
	}

	fileSize := uint32(len(data))
	fileID := md5.Sum(data)

	fmt.Fprintf(d.log, "→ UploadFile type=%s addr=0x%08X size=%d\n", fileType, addr, fileSize)

	// ── 1. Handshake ──────────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("uploadFile: %w", err)
	}
	if _, err := d.Handshake(); err != nil {
		return fmt.Errorf("uploadFile: handshake: %w", err)
	}

	// ── 2. GetDownloadStatus ──────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("uploadFile: %w", err)
	}
	status, err := d.GetDownloadStatus()
	if err != nil {
		return fmt.Errorf("uploadFile: getDownloadStatus: %w", err)
	}

	startOffset := uint32(0)

	switch status.Status {
	case downloadStatePrep, downloadStateActive:
		if status.FileID == fileID {
			startOffset = status.Offset
			fmt.Fprintf(d.log, "  resuming from offset %d\n", startOffset)
		}

	case downloadStateAHMI:
		if status.FileID == fileID {
			fmt.Fprintln(d.log, "  file already present on the device")
			d.reportProgress(cfg.OnProgress, int64(fileSize), int64(fileSize))
			return nil
		}
		if err := d.switchToDownloadMode(ctx); err != nil {
			return fmt.Errorf("uploadFile: switchState: %w", err)
		}

	default:
		return fmt.Errorf("uploadFile: invalid download state: 0x%02X", status.Status)
	}

	// ── 3. RequestDownload ────────────────────────────────────────────────────
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("uploadFile: %w", err)
	}
	dlResult, err := d.RequestDownload(addr, fileSize, fileID)
	if err != nil {
		return fmt.Errorf("uploadFile: requestDownload: %w", err)
	}

	maxPageSize := dlResult.MaxPageSize
	if maxPageSize == 0 {
		return fmt.Errorf("uploadFile: device returned maxPageSize=0")
	}

	if dlResult.Response == respProcessing {
		fmt.Fprintln(d.log, "  requestDownload: waiting for final response from device")
		if err := d.waitForDownloadReady(ctx); err != nil {
			return fmt.Errorf("uploadFile: requestDownload processing: %w", err)
		}
	} else if dlResult.Response != 0 {
		return fmt.Errorf("uploadFile: requestDownload rejected: 0x%08X", dlResult.Response)
	}

	// ── 4. Data transfer ──────────────────────────────────────────────────────
	offset := startOffset
	lastReportedPct := -1

	for offset < fileSize {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("uploadFile: %w", err)
		}

		end := min(offset+maxPageSize, fileSize)
		chunk := data[offset:end]

		if err := d.sendChunk(ctx, offset, chunk, cfg.chunkTimeout()); err != nil {
			return fmt.Errorf("uploadFile: sendChunk offset=%d: %w", offset, err)
		}

		offset = end

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
	if err := d.DownloadComplete(ctx); err != nil {
		fmt.Fprintf(d.log, "  DownloadComplete: %v (ignored)\n", err)
	}

	fmt.Fprintln(d.log, "✓ UploadFile completed")
	return nil
}

// ─────────────────────────────────────────────
// Upload internal helpers
// ─────────────────────────────────────────────

// switchToDownloadMode sends SWITCH_STATE with value 0x10 to exit AHMI mode.
func (d *Device) switchToDownloadMode(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintln(d.log, "→ SwitchState → download mode")

	cmd := NewCommand(CmdSwitchState, []byte{0x10})
	resp, err := d.sendWithRetry(cmd, CmdSwitchStateResp, 1*time.Second, 3)
	if err != nil {
		return err
	}

	if len(resp.Content) < 4 {
		return fmt.Errorf("SwitchState response too short")
	}
	code := binary.BigEndian.Uint32(resp.Content[0:4])
	if code != 0 {
		return fmt.Errorf("SwitchState failed with code 0x%08X", code)
	}
	return nil
}

// waitForDownloadReady waits until a REQUEST_DOWNLOAD_RESPONSE with code 0 is received.
func (d *Device) waitForDownloadReady(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := ReadResponse(d.serial, 5*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				continue
			}
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
			return fmt.Errorf("requestDownload: error code 0x%08X", code)
		}
	}
}

// sendChunk sends a data chunk and handles the "processing" pattern.
func (d *Device) sendChunk(ctx context.Context, offset uint32, chunk []byte, timeout time.Duration) error {
	padding := (4 - len(chunk)&3) & 3
	content := make([]byte, 4+len(chunk)+padding)
	binary.BigEndian.PutUint32(content[0:4], offset)
	copy(content[4:], chunk)

	cmd := NewCommand(CmdDownloadData, content)

	var lastErr error
	for range 3 {
		if err := ctx.Err(); err != nil {
			return err
		}

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
			lastErr = fmt.Errorf("DownloadData response too short")
			continue
		}

		code := binary.BigEndian.Uint32(resp.Content[0:4])
		switch code {
		case 0:
			return nil
		case respProcessing:
			if err := d.waitForChunkDone(ctx, timeout); err != nil {
				lastErr = err
				continue
			}
			return nil
		default:
			lastErr = fmt.Errorf("DownloadData: code 0x%08X", code)
			continue
		}
	}

	return fmt.Errorf("sendChunk: retries exhausted: %w", lastErr)
}

// waitForChunkDone waits for the device to confirm it processed the chunk.
func (d *Device) waitForChunkDone(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return ErrTimeout
		}

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
// DownloadComplete (full implementation with processing support)
// ─────────────────────────────────────────────

// DownloadComplete notifies the device that the transfer is complete.
// Overrides the simple version in device.go with support for the "processing" pattern.
func (d *Device) DownloadComplete(ctx context.Context) error {
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
		return fmt.Errorf("DownloadComplete response too short")
	}

	code := binary.BigEndian.Uint32(resp.Content[0:4])
	if code == 0 {
		return nil
	}
	if code != respProcessing {
		return fmt.Errorf("DownloadComplete: error 0x%08X", code)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := ReadResponse(d.serial, 1*time.Second)
		if err != nil {
			if errors.Is(err, ErrTimeout) {
				continue
			}
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
