package core

import (
	"encoding/binary"
	"fmt"
)

// CommandType represents a command type in the Minitela protocol.
// Response commands follow the pattern: type | 0x0040
type CommandType uint16

const (
	CmdHandshake             CommandType = 0x0080
	CmdHandshakeResponse     CommandType = 0x00C0
	CmdGetDownloadStatus     CommandType = 0x0085
	CmdGetDownloadStatusResp CommandType = 0x00C5
	CmdGetOffset             CommandType = 0x0086
	CmdGetOffsetResp         CommandType = 0x00C6
	CmdRequestDownload       CommandType = 0x0081
	CmdRequestDownloadResp   CommandType = 0x00C1
	CmdDownloadData          CommandType = 0x0082
	CmdDownloadDataResp      CommandType = 0x00C2
	CmdDownloadComplete      CommandType = 0x008F
	CmdDownloadCompleteResp  CommandType = 0x00CF
	CmdUploadData            CommandType = 0x0083
	CmdUploadDataResp        CommandType = 0x00C3
	CmdSwitchState           CommandType = 0x0071
	CmdSwitchStateResp       CommandType = 0x00B1
	CmdReboot                CommandType = 0x0070
	CmdRebootResp            CommandType = 0x00B0
	CmdSetRegister           CommandType = 0x0090
	CmdSetRegisterResp       CommandType = 0x00D0
)

func (c CommandType) String() string {
	names := map[CommandType]string{
		CmdHandshake:             "Handshake",
		CmdHandshakeResponse:     "HandshakeResponse",
		CmdGetDownloadStatus:     "GetDownloadStatus",
		CmdGetDownloadStatusResp: "GetDownloadStatusResponse",
		CmdGetOffset:             "GetOffset",
		CmdGetOffsetResp:         "GetOffsetResponse",
		CmdRequestDownload:       "RequestDownload",
		CmdRequestDownloadResp:   "RequestDownloadResponse",
		CmdDownloadData:          "DownloadData",
		CmdDownloadDataResp:      "DownloadDataResponse",
		CmdDownloadComplete:      "DownloadComplete",
		CmdDownloadCompleteResp:  "DownloadCompleteResponse",
		CmdUploadData:            "UploadData",
		CmdUploadDataResp:        "UploadDataResponse",
		CmdSwitchState:           "SwitchState",
		CmdSwitchStateResp:       "SwitchStateResponse",
		CmdReboot:                "Reboot",
		CmdRebootResp:            "RebootResponse",
		CmdSetRegister:           "SetRegister",
		CmdSetRegisterResp:       "SetRegisterResponse",
	}
	if name, ok := names[c]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(0x%04X)", uint16(c))
}

// IsResponse returns true if the CommandType corresponds to a device response.
func (c CommandType) IsResponse() bool {
	return c&0x00C0 == 0x00C0 || c&0x00B0 == 0x00B0
}

// ─────────────────────────────────────────────
// Frame layout:
//
//	[0x41, 0x48] startFlag  (2)
//	controlFlag             (2 BE): bits[14:0] = len(cmdType)+len(content), bit[15] = CRC enabled
//	cmdType                 (2 BE)
//	content                 (N)
//	crc16                   (2 BE, only if CRC enabled)
//	[0x4D, 0x49] endFlag    (2)
//
// ─────────────────────────────────────────────
var (
	frameStart = [2]byte{0x41, 0x48}
	frameEnd   = [2]byte{0x4D, 0x49}
)

// Command encapsulates a command ready to be sent.
type Command struct {
	Type       CommandType
	Content    []byte
	crcEnabled bool
}

// NewCommand creates a command without CRC.
func NewCommand(t CommandType, content []byte) *Command {
	return &Command{Type: t, Content: content}
}

// NewCommandWithCRC creates a command with CRC enabled.
func NewCommandWithCRC(t CommandType, content []byte) *Command {
	return &Command{Type: t, Content: content, crcEnabled: true}
}

// Frame builds the complete frame ready to be sent over serial.
func (c *Command) Frame() []byte {
	dataLen := 2 + len(c.Content) // cmdType (2) + content

	var controlFlag uint16 = uint16(dataLen)
	if c.crcEnabled {
		controlFlag |= 0x8000
	}

	// Pre-calculate size to avoid extra allocations.
	// The CRC field (2 bytes) is always present: filled with the real CRC or 0x0000.
	frameSize := 2 + 2 + 2 + len(c.Content) + 2 + 2 // start+ctrl+type+content+crc+end

	buf := make([]byte, 0, frameSize)

	buf = append(buf, frameStart[:]...)
	buf = binary.BigEndian.AppendUint16(buf, controlFlag)
	buf = binary.BigEndian.AppendUint16(buf, uint16(c.Type))
	buf = append(buf, c.Content...)

	if c.crcEnabled {
		// CRC covers: controlFlag + cmdType + content
		crcInput := buf[2:] // from controlFlag to end of current buffer
		crc := crc16(crcInput)
		buf = binary.BigEndian.AppendUint16(buf, crc)
	} else {
		// CRC disabled: 2 zero bytes
		buf = append(buf, 0x00, 0x00)
	}

	buf = append(buf, frameEnd[:]...)

	return buf
}

// ─────────────────────────────────────────────
// Response
// ─────────────────────────────────────────────

// Response represents a parsed response from the device.
type Response struct {
	Type    CommandType
	Content []byte

	// internal fields for validation
	crcEnabled bool
	crcOK      bool
}

// ParseFrame parses a raw frame into a Response.
// Expects to receive exactly one complete and valid frame.
func ParseFrame(raw []byte) (*Response, error) {
	if len(raw) < 8 {
		return nil, fmt.Errorf("frame too short: %d bytes", len(raw))
	}

	if raw[0] != frameStart[0] || raw[1] != frameStart[1] {
		return nil, fmt.Errorf("invalid start flag: %02X %02X", raw[0], raw[1])
	}

	controlFlag := binary.BigEndian.Uint16(raw[2:4])
	crcEnabled := (controlFlag & 0x8000) != 0
	dataLen := int(controlFlag & 0x7FFF) // bits[14:0]

	if dataLen < 2 {
		return nil, fmt.Errorf("invalid dataLen: %d", dataLen)
	}

	contentLen := dataLen - 2                      // dataLen includes the 2 cmdType bytes
	expectedSize := 2 + 2 + 2 + contentLen + 2 + 2 // start+ctrl+type+content+crc+end

	if len(raw) < expectedSize {
		return nil, fmt.Errorf("incomplete frame: need %d bytes, have %d", expectedSize, len(raw))
	}

	cmdType := CommandType(binary.BigEndian.Uint16(raw[4:6]))
	content := make([]byte, contentLen)
	copy(content, raw[6:6+contentLen])

	crcOK := true
	if crcEnabled {
		crcPayload := raw[2 : 6+contentLen] // controlFlag + cmdType + content
		expected := binary.BigEndian.Uint16(raw[6+contentLen : 8+contentLen])
		actual := crc16(crcPayload)
		crcOK = expected == actual
	}

	endOffset := 6 + contentLen + 2
	if raw[endOffset] != frameEnd[0] || raw[endOffset+1] != frameEnd[1] {
		return nil, fmt.Errorf("invalid end flag: %02X %02X", raw[endOffset], raw[endOffset+1])
	}

	return &Response{
		Type:       cmdType,
		Content:    content,
		crcEnabled: crcEnabled,
		crcOK:      crcOK,
	}, nil
}

// CRCValid returns whether the CRC was validated successfully.
// If the frame has no CRC enabled, returns true.
func (r *Response) CRCValid() bool {
	return r.crcOK
}

// ─────────────────────────────────────────────
// CRC-16/IBM (poly 0x8005, reflected input/output)
// Equivalent to the crc16() from the npm package 'crc'
// ─────────────────────────────────────────────

// crc16Table is a pre-computed lookup table for CRC-16/IBM (reflected)
var crc16Table = func() []uint16 {
	table := make([]uint16, 256)
	for i := range table {
		crc := uint16(i)
		for range 8 {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
		table[i] = crc
	}
	return table
}()

func crc16(data []byte) uint16 {
	var crc uint16 = 0x0000
	for _, b := range data {
		crc = (crc >> 8) ^ crc16Table[(crc^uint16(b))&0xFF]
	}
	return crc
}
