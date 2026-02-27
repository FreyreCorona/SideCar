// Package core implements the connection and exposes resources of the mini-screen (Minitela).
package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// Device representa una Minitela conectada por serial.
type Device struct {
	serial *SerialPort
	log    io.Writer
}

func NewDevice(port *SerialPort) *Device {
	return &Device{
		serial: port,
		log:    io.Discard,
	}
}

func (d *Device) SetLogger(w io.Writer) {
	if w == nil {
		d.log = io.Discard
		return
	}
	d.log = w
}

func (d *Device) Close() error {
	if d.serial == nil {
		return nil
	}
	return d.serial.Close()
}

// send envía un comando y espera exactamente una respuesta del tipo indicado.
func (d *Device) send(cmd *Command, expect CommandType, timeout time.Duration) (*Response, error) {
	if err := d.serial.Write(cmd.Frame()); err != nil {
		return nil, fmt.Errorf("send %s: %w", cmd.Type, err)
	}
	resp, err := ExpectResponse(d.serial, expect, timeout)
	if err != nil {
		return nil, fmt.Errorf("send %s → %s: %w", cmd.Type, expect, err)
	}
	return resp, nil
}

// sendWithRetry envía un comando y reintenta ante timeouts hasta maxRetries veces.
func (d *Device) sendWithRetry(cmd *Command, expect CommandType, timeout time.Duration, maxRetries int) (*Response, error) {
	var lastErr error
	for attempt := 0; maxRetries == 0 || attempt <= maxRetries; attempt++ {
		if err := d.serial.Write(cmd.Frame()); err != nil {
			return nil, fmt.Errorf("sendWithRetry write %s: %w", cmd.Type, err)
		}
		resp, err := ExpectResponse(d.serial, expect, timeout)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !errors.Is(err, ErrTimeout) {
			return nil, fmt.Errorf("sendWithRetry %s: %w", cmd.Type, err)
		}
		fmt.Fprintf(d.log, "  timeout en %s (intento %d/%d)\n", cmd.Type, attempt+1, maxRetries)
	}
	return nil, fmt.Errorf("sendWithRetry %s: %w", cmd.Type, lastErr)
}

// HandshakeResult contiene los datos de la respuesta al handshake.
type HandshakeResult struct {
	MaxPacketLength uint32
}

func (d *Device) Handshake() (*HandshakeResult, error) {
	fmt.Fprintln(d.log, "→ Handshake")
	cmd := NewCommand(CmdHandshake, nil)
	resp, err := d.sendWithRetry(cmd, CmdHandshakeResponse, 500*time.Millisecond, 3)
	if err != nil {
		return nil, err
	}
	if len(resp.Content) < 4 {
		return nil, fmt.Errorf("handshake response too short: %d bytes", len(resp.Content))
	}
	return &HandshakeResult{
		MaxPacketLength: binary.BigEndian.Uint32(resp.Content[0:4]),
	}, nil
}

// SetBrightness controla el brillo via registro de hardware.
// El JS original hardcodeaba el CRC — aquí lo calculamos correctamente.
func (d *Device) SetBrightness(level uint8) error {
	fmt.Fprintf(d.log, "→ SetBrightness %d\n", level)
	_, err := d.WriteNumRegisters([]NumRegister{
		{RegID: RegBrightness, Value: uint32(level)},
	})
	return err
}

func (d *Device) Wake() error {
	fmt.Fprintln(d.log, "→ Wake")
	return d.SetBrightness(100)
}

func (d *Device) Sleep() error {
	fmt.Fprintln(d.log, "→ Sleep")
	return d.SetBrightness(0)
}

func (d *Device) Reboot() error {
	fmt.Fprintln(d.log, "→ Reboot")
	cmd := NewCommand(CmdReboot, nil)
	_, err := d.send(cmd, CmdRebootResp, 2*time.Second)
	return err
}

// DownloadStatusResult contiene el estado del proceso de descarga activo.
type DownloadStatusResult struct {
	Status uint8
	FileID [16]byte
	Offset uint32
}

func (d *Device) GetDownloadStatus() (*DownloadStatusResult, error) {
	fmt.Fprintln(d.log, "→ GetDownloadStatus")
	cmd := NewCommand(CmdGetDownloadStatus, nil)
	resp, err := d.sendWithRetry(cmd, CmdGetDownloadStatusResp, 1*time.Second, 3)
	if err != nil {
		return nil, err
	}
	if len(resp.Content) < 21 {
		return nil, fmt.Errorf("GetDownloadStatus response demasiado corta: %d bytes", len(resp.Content))
	}
	result := &DownloadStatusResult{
		Status: resp.Content[0],
		Offset: binary.BigEndian.Uint32(resp.Content[17:21]),
	}
	copy(result.FileID[:], resp.Content[1:17])
	return result, nil
}

func (d *Device) GetOffset() (uint32, error) {
	fmt.Fprintln(d.log, "→ GetOffset")
	cmd := NewCommand(CmdGetOffset, nil)
	resp, err := d.send(cmd, CmdGetOffsetResp, 500*time.Millisecond)
	if err != nil {
		return 0, err
	}
	if len(resp.Content) < 4 {
		return 0, fmt.Errorf("GetOffset response demasiado corta: %d bytes", len(resp.Content))
	}
	return binary.BigEndian.Uint32(resp.Content[0:4]), nil
}

// RequestDownloadResult contiene los parámetros devueltos al abrir una sesión de descarga.
type RequestDownloadResult struct {
	MaxPageSize uint32
	Response    uint32
}

func (d *Device) RequestDownload(addr uint32, fileSize uint32, fileID [16]byte) (*RequestDownloadResult, error) {
	fmt.Fprintf(d.log, "→ RequestDownload addr=0x%08X size=%d\n", addr, fileSize)
	content := make([]byte, 24)
	binary.BigEndian.PutUint32(content[0:4], addr)
	binary.BigEndian.PutUint32(content[4:8], fileSize)
	copy(content[8:24], fileID[:])

	cmd := NewCommand(CmdRequestDownload, content)
	resp, err := d.send(cmd, CmdRequestDownloadResp, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if len(resp.Content) < 8 {
		return nil, fmt.Errorf("RequestDownload response demasiado corta: %d bytes", len(resp.Content))
	}
	return &RequestDownloadResult{
		MaxPageSize: binary.BigEndian.Uint32(resp.Content[0:4]),
		Response:    binary.BigEndian.Uint32(resp.Content[4:8]),
	}, nil
}

func (d *Device) SwitchState(payload []byte) error {
	fmt.Fprintf(d.log, "→ SwitchState payload=%X\n", payload)
	cmd := NewCommand(CmdSwitchState, payload)
	_, err := d.send(cmd, CmdSwitchStateResp, 1*time.Second)
	return err
}

// AutoConnect prueba todos los puertos seriales disponibles hasta encontrar una Minitela.
func AutoConnect(baud int) (*Device, error) {
	devices, err := FindSerialDevices()
	if err != nil {
		return nil, err
	}
	for _, path := range devices {
		fmt.Println("→ probing", path)
		port, err := OpenSerial(path, baud)
		if err != nil {
			fmt.Println("  ✗", err)
			continue
		}
		dev := NewDevice(port)
		if _, err := dev.Handshake(); err == nil {
			fmt.Println("  ✓ connected to", path)
			return dev, nil
		} else {
			fmt.Println("  ✗ handshake:", err)
			dev.Close()
		}
	}
	return nil, fmt.Errorf("no compatible minitela found")
}
