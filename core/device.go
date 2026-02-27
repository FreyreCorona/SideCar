// Package core implements the conection and expose resources of the mini-screen
package core

import (
	"fmt"
	"io"
	"time"
)

var handshakePayload = []byte{
	0x80, 0x01,
	0x00, 0x00,
}

var brightnessPayloadTemplate = []byte{
	0x80,
	0x09,
	0x00,
	0x90,
	0x80,
	0x00,
	0x07,
	0x00,
	0x00,
	0x00,
	0x00, // brightness
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

	frame := BuildFrame(handshakePayload)

	if err := d.serial.Write(frame); err != nil {
		return err
	}

	return ExpectACK(d.serial, 500*time.Millisecond)
}

func (d *Device) SetBrightness(level uint8) error {
	payload := make([]byte, len(brightnessPayloadTemplate))
	copy(payload, brightnessPayloadTemplate)
	payload[10] = level

	frame := BuildFrame(payload)

	fmt.Fprintf(d.log, "→ Brightness %d\n", level)

	if err := d.serial.Write(frame); err != nil {
		return err
	}

	return ExpectACK(d.serial, 500*time.Millisecond)
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
			fmt.Println(err.Error())
			continue
		}

		dev := NewDevice(port)

		if err := dev.Handshake(); err == nil {
			fmt.Println("✓ connected to", path)
			return dev, nil
		} else {
			fmt.Println(err.Error())
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
