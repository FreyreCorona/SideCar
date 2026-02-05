package main

import (
	"fmt"
	"io"
)

var handshakeCmd = []byte{
	0x41, 0x48,
	0x80, 0x01,
	0x00, 0x00,
	0xE2, 0xCB, 0x4D, 0x49,
}

var brightnessCmdTemplate = []byte{
	0x41, 0x48,
	0x80, 0x09,
	0x00,
	0x90,
	0x80,
	0x00,
	0x07,
	0x00,
	0x00,
	0x00,
	0x00, // ← BRIGHTNESS LEVEL (offset 12)
	0xE2,
	0xCB,
	0x4D,
	0x49,
}

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

func (d *Device) Wake() error {
	fmt.Fprintln(d.log, "→ Wake device")
	return d.SetBrightness(100)
}

func (d *Device) Sleep() error {
	fmt.Fprintln(d.log, "→ Sleep device")
	return d.SetBrightness(0)
}

func (d *Device) Handshake() error {
	fmt.Fprintln(d.log, "→ Handshake")

	if _, err := d.serial.Write(handshakeCmd); err != nil {
		return err
	}

	resp, err := ReadResponse(d.serial, 64)
	if err != nil {
		return err
	}

	fmt.Fprintf(d.log, "← Handshake OK: % X\n", resp)
	return nil
}

func (d *Device) SetBrightness(level uint8) error {
	if level > 100 {
		return fmt.Errorf("brightness out of range: %d", level)
	}

	cmd := make([]byte, len(brightnessCmdTemplate))
	copy(cmd, brightnessCmdTemplate)
	cmd[12] = level

	fmt.Fprintf(d.log, "→ Brightness %d\n", level)

	if _, err := d.serial.Write(cmd); err != nil {
		return err
	}

	return ExpectACK(d.serial)
}

func AutoConnect(baud int) (*Device, error) {
	devices, err := FindSerialDevices()
	if err != nil {
		return nil, err
	}

	for _, path := range devices {
		fmt.Println("→ probing", path)

		port, err := OpenSerial(path, baud)
		if err != nil {
			continue
		}

		dev := NewDevice(port)

		if err := dev.Handshake(); err == nil {
			fmt.Println("✓ connected to", path)
			return dev, nil
		}

		dev.Close()
	}

	return nil, fmt.Errorf("no compatible minitela found")
}

func (d *Device) Close() error {
	if d.serial == nil {
		return nil
	}
	return d.serial.Close()
}
