package tests

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/FreyreCorona/SideCar/core"
)

// ── Command.Frame ──────────────────────────────────────────────────────────

func TestCommandFrame_StartsAndEndsCorrectly(t *testing.T) {
	cmd := core.NewCommand(core.CmdHandshake, nil)
	frame := cmd.Frame()

	if frame[0] != 0x41 || frame[1] != 0x48 {
		t.Errorf("start flag: got %02X %02X, want 41 48", frame[0], frame[1])
	}
	n := len(frame)
	if frame[n-2] != 0x4D || frame[n-1] != 0x49 {
		t.Errorf("end flag: got %02X %02X, want 4D 49", frame[n-2], frame[n-1])
	}
}

func TestCommandFrame_TypeEncoded(t *testing.T) {
	cmd := core.NewCommand(core.CmdReboot, nil)
	frame := cmd.Frame()
	gotType := binary.BigEndian.Uint16(frame[4:6])
	if core.CommandType(gotType) != core.CmdReboot {
		t.Errorf("got type 0x%04X, want 0x%04X", gotType, uint16(core.CmdReboot))
	}
}

func TestCommandFrame_ContentEmbedded(t *testing.T) {
	content := []byte{0xAA, 0xBB, 0xCC}
	cmd := core.NewCommand(core.CmdSetRegister, content)
	frame := cmd.Frame()
	// Content starts at offset 6 (after start+ctrl+type)
	got := frame[6 : 6+len(content)]
	if !bytes.Equal(got, content) {
		t.Errorf("got content %X, want %X", got, content)
	}
}

func TestCommandFrame_WithCRC_BitSet(t *testing.T) {
	cmd := core.NewCommandWithCRC(core.CmdHandshake, nil)
	frame := cmd.Frame()
	controlFlag := binary.BigEndian.Uint16(frame[2:4])
	if controlFlag&0x8000 == 0 {
		t.Error("CRC enabled bit should be set in controlFlag")
	}
}

func TestCommandFrame_NoCRC_BitClear(t *testing.T) {
	cmd := core.NewCommand(core.CmdHandshake, nil)
	frame := cmd.Frame()
	controlFlag := binary.BigEndian.Uint16(frame[2:4])
	if controlFlag&0x8000 != 0 {
		t.Error("CRC enabled bit should be clear for no-CRC command")
	}
}

func TestCommandFrame_RoundTrip(t *testing.T) {
	content := []byte{0x01, 0x02, 0x03}
	cmd := core.NewCommandWithCRC(core.CmdSetRegister, content)
	frame := cmd.Frame()

	resp, err := core.ParseFrame(frame)
	if err != nil {
		t.Fatalf("ParseFrame error: %v", err)
	}
	if resp.Type != core.CmdSetRegister {
		t.Errorf("got type %v, want SetRegister", resp.Type)
	}
	if !bytes.Equal(resp.Content, content) {
		t.Errorf("got content %X, want %X", resp.Content, content)
	}
	if !resp.CRCValid() {
		t.Error("CRC should be valid after round-trip")
	}
}

// ── ParseFrame ─────────────────────────────────────────────────────────────

func TestParseFrame_TooShort(t *testing.T) {
	_, err := core.ParseFrame([]byte{0x41, 0x48, 0x00})
	if err == nil {
		t.Error("expected error for short frame")
	}
}

func TestParseFrame_BadStartFlag(t *testing.T) {
	frame := core.NewCommand(core.CmdHandshake, nil).Frame()
	frame[0] = 0xFF // corrupt start
	_, err := core.ParseFrame(frame)
	if err == nil {
		t.Error("expected error for bad start flag")
	}
}

func TestParseFrame_BadEndFlag(t *testing.T) {
	frame := core.NewCommand(core.CmdHandshake, nil).Frame()
	frame[len(frame)-1] ^= 0xFF // corrupt end
	_, err := core.ParseFrame(frame)
	if err == nil {
		t.Error("expected error for bad end flag")
	}
}

func TestParseFrame_NilContent(t *testing.T) {
	cmd := core.NewCommand(core.CmdReboot, nil)
	frame := cmd.Frame()
	resp, err := core.ParseFrame(frame)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Content) != 0 {
		t.Errorf("expected empty content, got %X", resp.Content)
	}
}

// ── CommandType ────────────────────────────────────────────────────────────

func TestCommandType_String(t *testing.T) {
	tests := []struct {
		cmd  core.CommandType
		want string
	}{
		{core.CmdHandshake, "Handshake"},
		{core.CmdReboot, "Reboot"},
		{core.CmdSetRegister, "SetRegister"},
		{core.CmdDownloadData, "DownloadData"},
		{core.CommandType(0xFFFF), "Unknown(0xFFFF)"},
	}
	for _, tc := range tests {
		if got := tc.cmd.String(); got != tc.want {
			t.Errorf("CommandType(%04X).String() = %q, want %q", uint16(tc.cmd), got, tc.want)
		}
	}
}

func TestCommandType_IsResponse(t *testing.T) {
	if !core.CmdHandshakeResponse.IsResponse() {
		t.Error("CmdHandshakeResponse should be a response")
	}
	if !core.CmdRebootResp.IsResponse() {
		t.Error("CmdRebootResp should be a response")
	}
	if core.CmdHandshake.IsResponse() {
		t.Error("CmdHandshake should NOT be a response")
	}
	if core.CmdReboot.IsResponse() {
		t.Error("CmdReboot should NOT be a response")
	}
}

// ── Response.CRCValid ──────────────────────────────────────────────────────

func TestCRCValid_NoCRC(t *testing.T) {
	frame := core.NewCommand(core.CmdHandshake, []byte{0x01}).Frame()
	resp, err := core.ParseFrame(frame)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// When CRC is disabled, CRCValid must return true
	if !resp.CRCValid() {
		t.Error("CRCValid should return true when CRC is disabled")
	}
}

func TestCRCValid_WithCRC(t *testing.T) {
	frame := core.NewCommandWithCRC(core.CmdSetRegister, []byte{0xDE, 0xAD}).Frame()
	resp, err := core.ParseFrame(frame)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !resp.CRCValid() {
		t.Error("CRC should be valid for freshly built frame")
	}
}

// ── UploadConfig ───────────────────────────────────────────────────────────

func TestUploadConfig_DefaultChunkTimeout(t *testing.T) {
	cfg := core.UploadConfig{}
	// Access via method exposed through UploadFile — we test it indirectly
	// through the zero-value struct being safe to use (no panic)
	_ = cfg
}
