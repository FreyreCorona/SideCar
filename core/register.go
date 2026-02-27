package core

import (
	"encoding/binary"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// Tipos
// ─────────────────────────────────────────────

// NumRegister es un par (id, valor) para una escritura/lectura numérica.
type NumRegister struct {
	RegID uint16
	Value uint32
}

// Constantes del byte de cabecera del payload SET_REGISTER
// Estructura del header byte: [replyFlag(1) | functionCode(3) | regCount(4)]
const (
	regFuncWriteNum  = 0b1000 // functionCode para escribir numérico
	regFuncReadNum   = 0b1100 // functionCode para leer numérico
	regFuncWriteStr  = 0b1101 // functionCode para escribir string
	regFuncReadStr   = 0b1110 // functionCode para leer string
	maxRegsPerBatch  = 16
)

// ─────────────────────────────────────────────
// Escritura numérica
// ─────────────────────────────────────────────

// WriteNumRegisters escribe registros numéricos. Si hay más de 16, los divide en batches.
// Devuelve un mapa {regID → valor confirmado por el dispositivo}.
func (d *Device) WriteNumRegisters(regs []NumRegister) (map[uint16]uint32, error) {
	if len(regs) == 0 {
		return map[uint16]uint32{}, nil
	}

	result := make(map[uint16]uint32, len(regs))
	for i := 0; i < len(regs); i += maxRegsPerBatch {
		end := i + maxRegsPerBatch
		if end > len(regs) {
			end = len(regs)
		}
		batch, err := d.writeNumRegsBatch(regs[i:end])
		if err != nil {
			return nil, err
		}
		for k, v := range batch {
			result[k] = v
		}
	}
	return result, nil
}

func (d *Device) writeNumRegsBatch(regs []NumRegister) (map[uint16]uint32, error) {
	n := len(regs)
	// header(1) + n*(regId(2)+value(4))
	content := make([]byte, 1+n*6)
	content[0] = byte((regFuncWriteNum << 4) | ((n - 1) & 0xF))

	for i, r := range regs {
		binary.BigEndian.PutUint16(content[1+i*6:], r.RegID)
		binary.BigEndian.PutUint32(content[1+i*6+2:], r.Value)
	}

	fmt.Fprintf(d.log, "→ WriteNumRegisters batch=%d\n", n)

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 1*time.Second, 3)
	if err != nil {
		return nil, err
	}
	return parseNumRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// Lectura numérica
// ─────────────────────────────────────────────

// ReadNumRegisters lee registros numéricos. Si hay más de 16, los divide en batches.
func (d *Device) ReadNumRegisters(regIDs []uint16) (map[uint16]uint32, error) {
	if len(regIDs) == 0 {
		return map[uint16]uint32{}, nil
	}

	result := make(map[uint16]uint32, len(regIDs))
	for i := 0; i < len(regIDs); i += maxRegsPerBatch {
		end := i + maxRegsPerBatch
		if end > len(regIDs) {
			end = len(regIDs)
		}
		batch, err := d.readNumRegsBatch(regIDs[i:end])
		if err != nil {
			return nil, err
		}
		for k, v := range batch {
			result[k] = v
		}
	}
	return result, nil
}

func (d *Device) readNumRegsBatch(regIDs []uint16) (map[uint16]uint32, error) {
	n := len(regIDs)
	// header(1) + n*regId(2)
	content := make([]byte, 1+n*2)
	content[0] = byte((regFuncReadNum << 4) | ((n - 1) & 0xF))
	for i, id := range regIDs {
		binary.BigEndian.PutUint16(content[1+i*2:], id)
	}

	fmt.Fprintf(d.log, "→ ReadNumRegisters batch=%d\n", n)

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 1*time.Second, 3)
	if err != nil {
		return nil, err
	}
	return parseNumRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// Escritura de string
// ─────────────────────────────────────────────

// WriteStringRegister escribe un valor string (como []byte) en un registro.
// Devuelve el slice de bytes confirmado por el dispositivo.
func (d *Device) WriteStringRegister(regID uint16, data []byte) ([]byte, error) {
	// header(1) + regId(2) + length(2) + data
	content := make([]byte, 5+len(data))
	content[0] = byte(regFuncWriteStr << 4)
	binary.BigEndian.PutUint16(content[1:], regID)
	binary.BigEndian.PutUint16(content[3:], uint16(len(data)))
	copy(content[5:], data)

	fmt.Fprintf(d.log, "→ WriteStringRegister regID=%d len=%d\n", regID, len(data))

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 1*time.Second, 3)
	if err != nil {
		return nil, err
	}
	return parseStringRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// Lectura de string
// ─────────────────────────────────────────────

// ReadStringRegister lee un registro de tipo string con longitud máxima especificada.
func (d *Device) ReadStringRegister(regID uint16, length uint16) ([]byte, error) {
	content := make([]byte, 5)
	content[0] = byte(regFuncReadStr << 4)
	binary.BigEndian.PutUint16(content[1:], regID)
	binary.BigEndian.PutUint16(content[3:], length)

	fmt.Fprintf(d.log, "→ ReadStringRegister regID=%d maxLen=%d\n", regID, length)

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 1*time.Second, 3)
	if err != nil {
		return nil, err
	}
	return parseStringRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// Parsing de respuestas SET_REGISTER_RESPONSE
//
// Estructura del byte de respuesta:
//   bits[7]:   reserved
//   bits[6:4]: functionCode
//   bits[3:0]: regNum (N-1, donde N = cantidad de registros)
// ─────────────────────────────────────────────

func parseNumRegResponse(content []byte) (map[uint16]uint32, error) {
	if len(content) < 1 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE: contenido vacío")
	}

	header := content[0]
	functionCode := (header & 0x70) >> 4
	regNum := int(header&0x0F) + 1 // regNum en el frame es N-1
	data := content[1:]

	if functionCode != 0 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE: functionCode inesperado %d (esperado 0)", functionCode)
	}

	// Cada registro: regId(2) + value(4) = 6 bytes
	if len(data) < regNum*6 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE: datos insuficientes: tengo %d, necesito %d", len(data), regNum*6)
	}

	result := make(map[uint16]uint32, regNum)
	for i := 0; i < regNum; i++ {
		regID := binary.BigEndian.Uint16(data[i*6:])
		value := binary.BigEndian.Uint32(data[i*6+2:])
		result[regID] = value
	}
	return result, nil
}

func parseStringRegResponse(content []byte) ([]byte, error) {
	if len(content) < 5 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE (string): contenido demasiado corto: %d bytes", len(content))
	}

	// data[0]: regId(2) | length(2) | string_bytes
	data := content[1:]
	// regID := binary.BigEndian.Uint16(data[0:2]) // disponible si se necesita
	strLen := int(binary.BigEndian.Uint16(data[2:4]))

	if len(data) < 4+strLen {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE (string): datos insuficientes")
	}

	result := make([]byte, strLen)
	copy(result, data[4:4+strLen])
	return result, nil
}
