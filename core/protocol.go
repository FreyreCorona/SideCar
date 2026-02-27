package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"time"
)

var (
	ErrTimeout    = errors.New("timeout waiting response")
	ErrShortFrame = errors.New("short frame")
	ErrNACK       = errors.New("device returned NACK")
)

var header = []byte{0x41, 0x48}

func CRC(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

func BuildFrame(payload []byte) []byte {
	frameLen := 2 + 1 + len(payload) + 4
	frame := make([]byte, frameLen)

	copy(frame[0:2], header)
	frame[2] = byte(len(payload))
	copy(frame[3:], payload)

	crc := crc32.ChecksumIEEE(frame[:3+len(payload)])
	binary.LittleEndian.PutUint32(frame[3+len(payload):], crc)

	return frame
}

func ReadResponse(p *SerialPort, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)

	buf := make([]byte, 0, 256)
	tmp := make([]byte, 64)

	for {
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}

		n, err := p.Read(tmp)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			continue
		}

		buf = append(buf, tmp[:n]...)

		i := bytes.Index(buf, header)
		if i == -1 {
			if len(buf) > 4096 {
				buf = buf[:0]
			}
			continue
		}

		if len(buf[i:]) < 3 {
			continue
		}

		payloadLen := int(buf[i+2])
		frameLen := 2 + 1 + payloadLen + 4

		if len(buf[i:]) < frameLen {
			continue
		}

		frame := buf[i : i+frameLen]

		dataEnd := i + 3 + payloadLen
		expected := binary.LittleEndian.Uint32(buf[dataEnd : dataEnd+4])
		actual := crc32.ChecksumIEEE(buf[i:dataEnd])

		if expected != actual {
			buf = buf[i+2:]
			continue
		}

		return frame, nil
	}
}

func ExpectACK(port *SerialPort, timeout time.Duration) error {
	resp, err := ReadResponse(port, timeout)
	if err != nil {
		return err
	}

	if len(resp) < 7 {
		return ErrShortFrame
	}

	status := resp[3]

	if status != 0x00 {
		return ErrNACK
	}

	return nil
}
