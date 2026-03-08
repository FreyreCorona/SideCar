package core

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ── crc16

func TestCRC16_KnownValues(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint16
	}{
		{"empty", []byte{}, 0x0000},
		{"single zero byte", []byte{0x00}, 0x0000},
		{"single 0xFF", []byte{0xFF}, 0x4040},
		{"0x01 0x02", []byte{0x01, 0x02}, 0x5180},
		// Known vector for CRC-16/IBM (poly 0x8005, reflected): "123456789" → 0xBB3D
		{"standard vector", []byte("123456789"), 0xBB3D},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := crc16(tc.data)
			if got != tc.want {
				t.Errorf("crc16(%X) = 0x%04X, want 0x%04X", tc.data, got, tc.want)
			}
		})
	}
}

// ── tryParseFrame ──────────────────────────────────────────────────────────

func buildValidFrame(cmdType CommandType, content []byte, withCRC bool) []byte {
	cmd := &Command{Type: cmdType, Content: content, crcEnabled: withCRC}
	return cmd.Frame()
}

func TestTryParseFrame_ValidNoCRC(t *testing.T) {
	frame := buildValidFrame(CmdHandshake, []byte{0x01, 0x02}, false)
	resp, consumed, err := tryParseFrame(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != CmdHandshake {
		t.Errorf("got type %v, want %v", resp.Type, CmdHandshake)
	}
	if !bytes.Equal(resp.Content, []byte{0x01, 0x02}) {
		t.Errorf("got content %X, want 0102", resp.Content)
	}
	if consumed <= 0 {
		t.Error("consumed must be positive")
	}
}

func TestTryParseFrame_ValidWithCRC(t *testing.T) {
	frame := buildValidFrame(CmdSetRegister, []byte{0xAA, 0xBB, 0xCC}, true)
	resp, _, err := tryParseFrame(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.CRCValid() {
		t.Error("CRC should be valid")
	}
}

func TestTryParseFrame_NeedMoreData(t *testing.T) {
	// Only start flag — not enough bytes
	_, _, err := tryParseFrame([]byte{0x41, 0x48, 0x00})
	if err != errNeedMore {
		t.Errorf("expected errNeedMore, got %v", err)
	}
}

func TestTryParseFrame_EmptyBuffer(t *testing.T) {
	_, _, err := tryParseFrame([]byte{})
	if err != errNeedMore {
		t.Errorf("expected errNeedMore, got %v", err)
	}
}

func TestTryParseFrame_BadCRC(t *testing.T) {
	frame := buildValidFrame(CmdHandshake, []byte{0x01}, true)
	// Corrupt the CRC bytes
	frame[len(frame)-4] ^= 0xFF
	_, _, err := tryParseFrame(frame)
	if err == nil {
		t.Error("expected CRC error, got nil")
	}
}

func TestTryParseFrame_SkipsJunk(t *testing.T) {
	junk := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frame := buildValidFrame(CmdReboot, nil, false)
	buf := append(junk, frame...)
	resp, consumed, err := tryParseFrame(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Type != CmdReboot {
		t.Errorf("got type %v, want Reboot", resp.Type)
	}
	_ = consumed
}

// ── discardUntilStartFlag ──────────────────────────────────────────────────

func TestDiscardUntilStartFlag_Found(t *testing.T) {
	buf := []byte{0xDE, 0xAD, 0x41, 0x48, 0x01, 0x02}
	got := discardUntilStartFlag(buf)
	want := []byte{0x41, 0x48, 0x01, 0x02}
	if !bytes.Equal(got, want) {
		t.Errorf("got %X, want %X", got, want)
	}
}

func TestDiscardUntilStartFlag_NotFound(t *testing.T) {
	buf := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	got := discardUntilStartFlag(buf)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %X", got)
	}
}

func TestDiscardUntilStartFlag_AtStart(t *testing.T) {
	buf := []byte{0x41, 0x48, 0x99}
	got := discardUntilStartFlag(buf)
	if !bytes.Equal(got, buf) {
		t.Errorf("got %X, want %X", got, buf)
	}
}

// ── parseNumRegResponse ────────────────────────────────────────────────────

func TestParseNumRegResponse_Single(t *testing.T) {
	// header(1) + regID(2) + value(4)
	// functionCode=0, regNum=0 → N=1
	content := make([]byte, 7)
	content[0] = 0x00                               // functionCode=0, regNum-1=0
	binary.BigEndian.PutUint16(content[1:], 0x0438) // regID
	binary.BigEndian.PutUint32(content[3:], 75)     // value
	result, err := parseNumRegResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val, ok := result[0x0438]; !ok || val != 75 {
		t.Errorf("expected result[0x0438]=75, got %v", result)
	}
}

func TestParseNumRegResponse_Multiple(t *testing.T) {
	// N=3 registers: header regNum-1=2 → content[0] = 0x02
	content := make([]byte, 1+3*6)
	content[0] = 0x02
	regs := []struct {
		id  uint16
		val uint32
	}{
		{1080, 45},
		{1081, 30},
		{1082, 90},
	}
	for i, r := range regs {
		binary.BigEndian.PutUint16(content[1+i*6:], r.id)
		binary.BigEndian.PutUint32(content[1+i*6+2:], r.val)
	}
	result, err := parseNumRegResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range regs {
		if val, ok := result[r.id]; !ok || val != r.val {
			t.Errorf("result[%d] = %d, want %d", r.id, val, r.val)
		}
	}
}

func TestParseNumRegResponse_EmptyContent(t *testing.T) {
	_, err := parseNumRegResponse([]byte{})
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestParseNumRegResponse_TruncatedData(t *testing.T) {
	// Header says 2 registers but only 1 register worth of data
	content := make([]byte, 1+6) // header + 1 reg, but says 2
	content[0] = 0x01            // regNum-1=1 → N=2
	_, err := parseNumRegResponse(content)
	if err == nil {
		t.Error("expected error for truncated data")
	}
}

// ── parseStringRegResponse ─────────────────────────────────────────────────

func TestParseStringRegResponse_Valid(t *testing.T) {
	str := []byte("hello")
	// content[0] = header, content[1:3] = regID, content[3:5] = length, content[5:] = data
	content := make([]byte, 5+len(str))
	// content[0] is the outer header (skipped in parsing)
	binary.BigEndian.PutUint16(content[1:], 0x0010) // regID
	binary.BigEndian.PutUint16(content[3:], uint16(len(str)))
	copy(content[5:], str)

	result, err := parseStringRegResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, str) {
		t.Errorf("got %q, want %q", result, str)
	}
}

func TestParseStringRegResponse_TooShort(t *testing.T) {
	_, err := parseStringRegResponse([]byte{0x00, 0x00})
	if err == nil {
		t.Error("expected error for short content")
	}
}

func TestParseStringRegResponse_Empty(t *testing.T) {
	content := make([]byte, 5) // header + regID + length(0)
	result, err := parseStringRegResponse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %X", result)
	}
}
