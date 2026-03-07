package core

import (
	"bytes"
	"fmt"
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

// FindSerialDevices returns all available serial ports on the system.
// Uses go.bug.st/serial's built-in port enumeration which works on both
// Linux (/dev/ttyUSB*, /dev/ttyACM*) and Windows (COM1, COM3, ...).
func FindSerialDevices() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("listing serial ports: %w", err)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("no serial devices found")
	}
	return ports, nil
}
