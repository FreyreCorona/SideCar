package core

import (
	"bytes"
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
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	p, err := serial.Open(device, mode)
	if err != nil {
		return nil, err
	}

	if err := p.SetReadTimeout(200 * time.Millisecond); err != nil {
		p.Close()
		return nil, err
	}

	time.Sleep(300 * time.Millisecond)
	drain(p, 500*time.Millisecond)

	return &SerialPort{port: p}, nil
}

func drain(p serial.Port, duration time.Duration) {
	deadline := time.Now().Add(duration)
	buf := make([]byte, 256)

	for time.Now().Before(deadline) {
		if _, err := p.Read(buf); err != nil {
			return
		}
	}
}

func (s *SerialPort) Write(data []byte) error {
	total := 0

	for total < len(data) {
		n, err := s.port.Write(data[total:])
		if err != nil {
			return err
		}
		total += n
	}

	return nil
}

func (s *SerialPort) Read(buf []byte) (int, error) {
	return s.port.Read(buf)
}

func (s *SerialPort) ReadUntil(delim byte, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 0, 128)
	tmp := make([]byte, 64)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for delimiter")
		}

		n, err := s.port.Read(tmp)
		if err != nil {
			return nil, err
		}

		if n == 0 {
			continue
		}

		buf = append(buf, tmp[:n]...)

		if i := bytes.IndexByte(buf, delim); i != -1 {
			return buf[:i+1], nil
		}

		if len(buf) > 4096 {
			return nil, fmt.Errorf("buffer overflow before delimiter")
		}
	}
}

func (s *SerialPort) Close() error {
	return s.port.Close()
}

func FindSerialDevices() ([]string, error) {
	patterns := []string{
		"/dev/ttyACM*",
		"/dev/ttyUSB*",
		"/dev/serial/by-id/*",
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
