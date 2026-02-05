package main

import (
	"fmt"
	"path/filepath"
	"time"

	"go.bug.st/serial"
)

type SerialPort struct {
	port serial.Port
}

func OpenSerial(device string, baud int) (*SerialPort, error) {
	mode := &serial.Mode{
		BaudRate: baud,
	}

	p, err := serial.Open(device, mode)
	if err != nil {
		return nil, err
	}

	if err := p.SetReadTimeout(500 * time.Millisecond); err != nil {
		p.Close()
		return nil, err
	}

	return &SerialPort{port: p}, nil
}

func (s *SerialPort) Write(data []byte) (int, error) {
	n, err := s.port.Write(data)
	if err != nil {
		return 0, err
	}
	if n != len(data) {
		return 0, fmt.Errorf("short write: %d/%d", n, len(data))
	}
	return n, err
}

func (s *SerialPort) Read(buf []byte) (int, error) {
	return s.port.Read(buf)
}

func (s *SerialPort) Close() error {
	return s.port.Close()
}

func FindSerialDevices() ([]string, error) {
	patterns := []string{
		"/dev/ttyACM*",
		"/dev/ttyUSB*",
	}

	var devices []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		devices = append(devices, matches...)
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no serial devices found")
	}
	return devices, nil
}
