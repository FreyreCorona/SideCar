package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

var (
	ErrTimeout   = errors.New("timeout waiting response")
	ErrBadHeader = errors.New("invalid response header")
)

var (
	header  = []byte{0x41, 0x48}
	ErrNACK = errors.New("device returned NACK")
)

func CRC(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// BuildFrame [DATA][CRC32]
func BuildFrame(payload []byte) []byte {
	crc := crc32.ChecksumIEEE(payload)

	frame := make([]byte, len(payload)+4)
	copy(frame, payload)

	binary.LittleEndian.PutUint32(frame[len(payload):], crc)
	return frame
}

func ReadResponse(r io.Reader, max int) ([]byte, error) {
	buf := make([]byte, max)
	deadline := time.Now().Add(500 * time.Millisecond)

	for {
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}

		n, err := r.Read(buf)
		if err != nil {
			return nil, err
		}

		if n < 2 {
			continue
		}

		if buf[0] != header[0] || buf[1] != header[1] {
			return nil, ErrBadHeader
		}

		return buf[:n], nil
	}
}

func ExpectACK(port *SerialPort) error {
	resp, err := ReadResponse(port, 64)
	if err != nil {
		return err
	}

	// Header ya validado en ReadResponse
	if len(resp) < 3 {
		return fmt.Errorf("short response")
	}

	status := resp[2]

	if status != 0x00 {
		return ErrNACK
	}

	return nil
}
