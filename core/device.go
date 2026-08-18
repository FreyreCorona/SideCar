// Package core implements the connection and exposes resources of the mini-screen (Minitela).
package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Device represents a Minitela connected over serial.
type Device struct {
	mu     sync.Mutex
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

// WriteRaw writes raw bytes to the serial port.
func (d *Device) WriteRaw(data []byte) error {
	return d.serial.Write(data)
}

// ReadRaw reads available bytes from the serial port into buf.
// Returns the number of bytes read.
func (d *Device) ReadRaw(buf []byte) int {
	total := 0
	for {
		n, err := d.serial.Read(buf[total:])
		if err != nil || n == 0 {
			break
		}
		total += n
		if total >= len(buf) {
			break
		}
	}
	return total
}

// SerialPort returns the underlying serial port for raw access.
func (d *Device) SerialPort() *SerialPort {
	return d.serial
}

// Lock acquires the device mutex, preventing concurrent access by the daemon
// and upload paths. Callers must pair with Unlock.
func (d *Device) Lock() {
	d.mu.Lock()
}

// Unlock releases the device mutex.
func (d *Device) Unlock() {
	d.mu.Unlock()
}

// send sends a command and waits for exactly one response of the expected type.
func (d *Device) send(cmd *Command, expect CommandType, timeout time.Duration) (*Response, error) {
	d.serial.Drain()
	if err := d.serial.Write(cmd.Frame()); err != nil {
		return nil, fmt.Errorf("send %s: %w", cmd.Type, err)
	}
	time.Sleep(200 * time.Millisecond)
	resp, err := ExpectResponse(d.serial, expect, timeout)
	if err != nil {
		return nil, fmt.Errorf("send %s → %s: %w", cmd.Type, expect, err)
	}
	return resp, nil
}

// sendWithRetry sends a command and retries on recoverable errors (timeout
// and unexpected type due to stale responses) up to maxRetries times.
func (d *Device) sendWithRetry(cmd *Command, expect CommandType, timeout time.Duration, maxRetries int) (*Response, error) {
	var lastErr error
	for attempt := 0; maxRetries == 0 || attempt <= maxRetries; attempt++ {
		if debug {
			fmt.Fprintf(os.Stderr, "[sendWithRetry] attempt=%d cmd=%s expect=%s\n", attempt, cmd.Type, expect)
		}
		d.serial.Drain()
		if err := d.serial.Write(cmd.Frame()); err != nil {
			return nil, fmt.Errorf("sendWithRetry write %s: %w", cmd.Type, err)
		}
		if debug {
			fmt.Fprintf(os.Stderr, "[sendWithRetry] wrote %d bytes, sleeping 200ms\n", len(cmd.Frame()))
		}
		time.Sleep(200 * time.Millisecond)
		resp, err := ExpectResponse(d.serial, expect, timeout)
		if err == nil {
			if debug {
				fmt.Fprintf(os.Stderr, "[sendWithRetry] got response: type=%s\n", resp.Type)
			}
			return resp, nil
		}
		lastErr = err
		if debug {
			fmt.Fprintf(os.Stderr, "[sendWithRetry] error: %v\n", err)
		}
		if !errors.Is(err, ErrTimeout) {
			return nil, fmt.Errorf("sendWithRetry %s: %w", cmd.Type, err)
		}
		if maxRetries == 0 {
			fmt.Fprintf(d.log, "  retry on %s: %v (attempt %d)\n", cmd.Type, err, attempt+1)
		} else {
			fmt.Fprintf(d.log, "  retry on %s: %v (attempt %d/%d)\n", cmd.Type, err, attempt+1, maxRetries+1)
		}
	}
	return nil, fmt.Errorf("sendWithRetry %s: %w", cmd.Type, lastErr)
}

// HandshakeResult holds the data from the handshake response.
type HandshakeResult struct {
	MaxPacketLength uint32
}

func (d *Device) Handshake() (*HandshakeResult, error) {
	fmt.Fprintln(d.log, "→ Handshake")
	cmd := NewCommand(CmdHandshake, nil)
	resp, err := d.sendWithRetry(cmd, CmdHandshakeResponse, 2*time.Second, 5)
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

// SetBrightness controls brightness via the hardware register.
// The original JS hardcoded the CRC — here we calculate it correctly.
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

// DownloadStatusResult holds the state of the active download process.
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
		return nil, fmt.Errorf("GetDownloadStatus response too short: %d bytes", len(resp.Content))
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
		return 0, fmt.Errorf("GetOffset response too short: %d bytes", len(resp.Content))
	}
	return binary.BigEndian.Uint32(resp.Content[0:4]), nil
}

// RequestDownloadResult holds the parameters returned when opening a download session.
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
		return nil, fmt.Errorf("RequestDownload response too short: %d bytes", len(resp.Content))
	}
	return &RequestDownloadResult{
		MaxPageSize: binary.BigEndian.Uint32(resp.Content[0:4]),
		Response:    binary.BigEndian.Uint32(resp.Content[4:8]),
	}, nil
}

func (d *Device) SwitchState(payload []byte) error {
	fmt.Fprintf(d.log, "→ SwitchState payload=%X\n", payload)
	cmd := NewCommand(CmdSwitchState, payload)
	_, err := d.sendWithRetry(cmd, CmdSwitchStateResp, 2*time.Second, 5)
	return err
}

// Connect opens a specific serial port and verifies it is a Minitela via handshake.
func Connect(port string, baud int) (*Device, error) {
	p, err := OpenSerial(port, baud)
	if err != nil {
		return nil, err
	}
	dev := NewDevice(p)
	if _, err := dev.Handshake(); err != nil {
		p.Close()
		return nil, err
	}
	return dev, nil
}

// AutoConnect probes all available serial ports until it finds a Minitela.
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
