package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTimeout        = errors.New("timeout waiting for response")
	ErrShortFrame     = errors.New("frame too short")
	ErrBadEndFlag     = errors.New("invalid end flag")
	ErrBadCRC         = errors.New("CRC mismatch")
	ErrUnexpectedType = errors.New("unexpected command type")
)

// ReadResponse lee del puerto hasta encontrar un frame válido o agotar el timeout.
// Busca el start flag, valida estructura, CRC y end flag.
func ReadResponse(p *SerialPort, timeout time.Duration) (*Response, error) {
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

		resp, consumed, err := tryParseFrame(buf)
		if err == errNeedMore {
			// No tenemos suficientes bytes aún, seguir leyendo
			if len(buf) > 4096 {
				// Descartamos hasta el próximo start flag para no crecer sin límite
				buf = discardUntilStartFlag(buf[1:])
			}
			continue
		}

		if err != nil {
			// Frame inválido en esta posición, avanzamos y buscamos el siguiente start flag
			buf = discardUntilStartFlag(buf[1:])
			continue
		}

		// Frame válido
		_ = consumed
		return resp, nil
	}
}

// errNeedMore indica que el buffer no tiene suficientes bytes todavía.
var errNeedMore = errors.New("need more data")

// tryParseFrame intenta parsear un frame desde el inicio del buffer.
// Devuelve (response, bytesConsumed, error).
func tryParseFrame(buf []byte) (*Response, int, error) {
	// Buscar start flag
	i := bytes.Index(buf, frameStart[:])
	if i == -1 {
		return nil, 0, errNeedMore
	}
	buf = buf[i:]

	// Necesitamos al menos: start(2) + control(2) + type(2) + crc(2) + end(2) = 10 bytes mínimo
	if len(buf) < 10 {
		return nil, 0, errNeedMore
	}

	controlFlag := binary.BigEndian.Uint16(buf[2:4])
	dataLen := int(controlFlag & 0x7FFF)

	if dataLen < 2 {
		return nil, 0, errors.New("dataLen inválido")
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
		return nil, 0, ErrBadCRC
	}

	return resp, i + frameSize, nil
}

// discardUntilStartFlag descarta bytes hasta encontrar el start flag.
func discardUntilStartFlag(buf []byte) []byte {
	i := bytes.Index(buf, frameStart[:])
	if i == -1 {
		return buf[:0]
	}
	return buf[i:]
}

// ExpectResponse lee una respuesta y valida que sea del tipo esperado.
func ExpectResponse(p *SerialPort, expected CommandType, timeout time.Duration) (*Response, error) {
	resp, err := ReadResponse(p, timeout)
	if err != nil {
		return nil, err
	}

	if resp.Type != expected {
		return nil, fmt.Errorf("%w: got %s, want %s", ErrUnexpectedType, resp.Type, expected)
	}

	return resp, nil
}
