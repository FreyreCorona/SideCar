package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
)

var (
	ErrTimeout        = errors.New("timeout waiting for response")
	ErrShortFrame     = errors.New("frame too short")
	ErrBadEndFlag     = errors.New("invalid end flag")
	ErrBadCRC         = errors.New("CRC mismatch")
	ErrUnexpectedType = errors.New("unexpected command type")
)

// ReadResponse reads from the port until it finds a valid frame or the timeout expires.
// Searches for the start flag, validates structure, CRC and end flag.
func ReadResponse(p *SerialPort, timeout time.Duration) (*Response, error) {
	deadline := time.Now().Add(timeout)
	tmp := make([]byte, 512)

	for {
		if time.Now().After(deadline) {
			// Clear buffer on timeout to prevent unbounded growth
			p.buf = p.buf[:0]
			return nil, ErrTimeout
		}

		n, err := p.Read(tmp)
		if err != nil {
			p.buf = p.buf[:0]
			return nil, err
		}

		if n > 0 {
			// Prevent buffer from growing beyond 16KB (reasonable for serial frames)
			if len(p.buf)+n > 16384 {
				p.buf = discardUntilStartFlag(p.buf)
			}
			p.buf = append(p.buf, tmp[:n]...)
		}

		resp, consumed, err := tryParseFrame(p.buf)
		if err == errNeedMore {
			const maxBufSize = 16384
			if len(p.buf) > maxBufSize {
				p.buf = discardUntilStartFlag(p.buf)
			}
			if n == 0 {
				time.Sleep(10 * time.Millisecond)
			}
			continue
		}

		if err != nil {
			p.buf = discardUntilStartFlag(p.buf)
			continue
		}

		// Success: remove consumed bytes from the persistent buffer
		p.buf = p.buf[consumed:]
		return resp, nil
	}
}

// errNeedMore indicates the buffer does not yet have enough bytes.
var errNeedMore = errors.New("need more data")

// tryParseFrame attempts to parse a frame from the beginning of the buffer.
// Returns (response, bytesConsumed, error).
func tryParseFrame(buf []byte) (*Response, int, error) {
	// Look for start flag
	i := bytes.Index(buf, frameStart[:])
	if i == -1 {
		return nil, 0, errNeedMore
	}
	buf = buf[i:]

	// Minimum needed: start(2) + control(2) + type(2) + crc(2) + end(2) = 10 bytes
	if len(buf) < 10 {
		return nil, 0, errNeedMore
	}

	controlFlag := binary.BigEndian.Uint16(buf[2:4])
	dataLen := int(controlFlag & 0x7FFF)

	if dataLen < 2 {
		return nil, 0, errors.New("invalid dataLen")
	}

	contentLen := dataLen - 2
	frameSize := 2 + 2 + 2 + contentLen + 2 + 2 // start+ctrl+type+content+crc+end

	if len(buf) < frameSize {
		return nil, 0, errNeedMore
	}

	frame := buf[:frameSize]
	resp, err := ParseFrame(frame)
	if err != nil {
		return nil, 0, err
	}

	if !resp.CRCValid() {
		// Some device firmware sends CRC-enabled responses with 0x0000 as
		// the CRC value.  Treat a CRC field of 0x0000 as "CRC not computed"
		// and accept the frame, since rejecting it causes timeouts in normal
		// operation.  Non-zero invalid CRCs are still rejected.
		// The CRC field sits at buf[6+contentLen : 8+contentLen].
		contentLen = dataLen - 2
		crcField := binary.BigEndian.Uint16(frame[6+contentLen:])
		if resp.crcEnabled && crcField == 0x0000 {
			resp.crcOK = true
		} else {
			return nil, 0, ErrBadCRC
		}
	}

	return resp, i + frameSize, nil
}

// discardUntilStartFlag discards bytes until the start flag is found.
func discardUntilStartFlag(buf []byte) []byte {
	i := bytes.Index(buf, frameStart[:])
	if i == -1 {
		return buf[:0]
	}
	return buf[i:]
}

// ExpectResponse reads from the port until a frame of the expected type is
// received, discarding any unexpected frames (e.g. progress/status updates
// sent by the device during long operations like flash erase).
func ExpectResponse(p *SerialPort, expected CommandType, timeout time.Duration) (*Response, error) {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, ErrTimeout
		}

		resp, err := ReadResponse(p, remaining)
		if err != nil {
			return nil, err
		}

		if resp.Type == expected {
			return resp, nil
		}
	}
}
