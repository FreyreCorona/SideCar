package core

import (
	"encoding/binary"
	"fmt"
	"maps"
	"time"
)

// ─────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────

// NumRegister is a (id, value) pair for a numeric read/write operation.
type NumRegister struct {
	RegID uint16
	Value uint32
}

// Constants for the header byte of the SET_REGISTER payload.
// Header byte structure: [replyFlag(1) | functionCode(3) | regCount(4)]
const (
	regFuncWriteNum = 0b1000 // functionCode for numeric write
	regFuncReadNum  = 0b1100 // functionCode for numeric read
	regFuncWriteStr = 0b1101 // functionCode for string write
	regFuncReadStr  = 0b1110 // functionCode for string read
	maxRegsPerBatch = 16
)

// ─────────────────────────────────────────────
// Numeric write
// ─────────────────────────────────────────────

// WriteNumRegisters writes numeric registers. If there are more than 16, splits them into batches.
// Returns a map {regID → value confirmed by the device}.
func (d *Device) WriteNumRegisters(regs []NumRegister) (map[uint16]uint32, error) {
	if len(regs) == 0 {
		return map[uint16]uint32{}, nil
	}

	result := make(map[uint16]uint32, len(regs))
	for i := 0; i < len(regs); i += maxRegsPerBatch {
		end := min(i+maxRegsPerBatch, len(regs))
		batch, err := d.writeNumRegsBatch(regs[i:end])
		if err != nil {
			return nil, err
		}
		maps.Copy(result, batch)
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
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 2*time.Second, 5)
	if err != nil {
		return nil, err
	}
	return parseNumRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// Numeric read
// ─────────────────────────────────────────────

// ReadNumRegisters reads numeric registers. If there are more than 16, splits them into batches.
func (d *Device) ReadNumRegisters(regIDs []uint16) (map[uint16]uint32, error) {
	if len(regIDs) == 0 {
		return map[uint16]uint32{}, nil
	}

	result := make(map[uint16]uint32, len(regIDs))
	for i := 0; i < len(regIDs); i += maxRegsPerBatch {
		end := min(i+maxRegsPerBatch, len(regIDs))
		batch, err := d.readNumRegsBatch(regIDs[i:end])
		if err != nil {
			return nil, err
		}
		maps.Copy(result, batch)
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
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 2*time.Second, 5)
	if err != nil {
		return nil, err
	}
	return parseNumRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// String write
// ─────────────────────────────────────────────

// WriteStringRegister writes a string value (as []byte) to a register.
// Returns the byte slice confirmed by the device.
func (d *Device) WriteStringRegister(regID uint16, data []byte) ([]byte, error) {
	// header(1) + regId(2) + length(2) + data
	content := make([]byte, 5+len(data))
	content[0] = byte(regFuncWriteStr << 4)
	binary.BigEndian.PutUint16(content[1:], regID)
	binary.BigEndian.PutUint16(content[3:], uint16(len(data)))
	copy(content[5:], data)

	fmt.Fprintf(d.log, "→ WriteStringRegister regID=%d len=%d\n", regID, len(data))

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 2*time.Second, 5)
	if err != nil {
		return nil, err
	}
	return parseStringRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// String read
// ─────────────────────────────────────────────

// ReadStringRegister reads a string register with the specified maximum length.
func (d *Device) ReadStringRegister(regID uint16, length uint16) ([]byte, error) {
	content := make([]byte, 5)
	content[0] = byte(regFuncReadStr << 4)
	binary.BigEndian.PutUint16(content[1:], regID)
	binary.BigEndian.PutUint16(content[3:], length)

	fmt.Fprintf(d.log, "→ ReadStringRegister regID=%d maxLen=%d\n", regID, length)

	cmd := NewCommand(CmdSetRegister, content)
	resp, err := d.sendWithRetry(cmd, CmdSetRegisterResp, 2*time.Second, 5)
	if err != nil {
		return nil, err
	}
	return parseStringRegResponse(resp.Content)
}

// ─────────────────────────────────────────────
// SET_REGISTER_RESPONSE parsing
//
// Response byte structure:
//   bits[7]:   reserved
//   bits[6:4]: functionCode
//   bits[3:0]: regNum (N-1, where N = number of registers)
// ─────────────────────────────────────────────

func parseNumRegResponse(content []byte) (map[uint16]uint32, error) {
	if len(content) < 1 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE: empty content")
	}

	header := content[0]
	functionCode := (header & 0x70) >> 4
	regNum := int(header&0x0F) + 1 // regNum in the frame is N-1
	data := content[1:]

	// For write ACKs the device may return functionCode=2 (not 0).
	// The Minipanel app does not validate functionCode for writes —
	// it only checks commandType==0x00D0.  Accept any functionCode;
	// only parse numeric data when functionCode==0.
	if functionCode != 0 {
		return map[uint16]uint32{}, nil
	}

	// Each register: regId(2) + value(4) = 6 bytes
	if len(data) < regNum*6 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE: insufficient data: have %d, need %d", len(data), regNum*6)
	}

	result := make(map[uint16]uint32, regNum)
	for i := range regNum {
		regID := binary.BigEndian.Uint16(data[i*6:])
		value := binary.BigEndian.Uint32(data[i*6+2:])
		result[regID] = value
	}
	return result, nil
}

func parseStringRegResponse(content []byte) ([]byte, error) {
	if len(content) < 5 {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE (string): content too short: %d bytes", len(content))
	}

	// data[0]: regId(2) | length(2) | string_bytes
	data := content[1:]
	// regID := binary.BigEndian.Uint16(data[0:2]) // available if needed
	strLen := int(binary.BigEndian.Uint16(data[2:4]))

	if len(data) < 4+strLen {
		return nil, fmt.Errorf("SET_REGISTER_RESPONSE (string): insufficient data")
	}

	result := make([]byte, strLen)
	copy(result, data[4:4+strLen])
	return result, nil
}
