package main

import (
	"strings"
	"testing"

	"github.com/FreyreCorona/SideCar/metrics"
)

// ── parseRegs ─────────────────────────────────────────────────────────────

func TestParseRegs_SingleDecimal(t *testing.T) {
	regs, err := parseRegs("1080=75")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regs) != 1 || regs[0].RegID != 1080 || regs[0].Value != 75 {
		t.Errorf("got %+v", regs)
	}
}

func TestParseRegs_SingleHex(t *testing.T) {
	regs, err := parseRegs("0x0438=0x4B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regs) != 1 || regs[0].RegID != 0x0438 || regs[0].Value != 0x4B {
		t.Errorf("got %+v", regs)
	}
}

func TestParseRegs_Multiple(t *testing.T) {
	regs, err := parseRegs("1080=75,1081=40,1082=90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regs) != 3 {
		t.Fatalf("got %d regs, want 3", len(regs))
	}
	expected := [][2]uint32{{1080, 75}, {1081, 40}, {1082, 90}}
	for i, e := range expected {
		if uint32(regs[i].RegID) != e[0] || regs[i].Value != e[1] {
			t.Errorf("regs[%d] = {%d,%d}, want {%d,%d}", i, regs[i].RegID, regs[i].Value, e[0], e[1])
		}
	}
}

func TestParseRegs_WithSpaces(t *testing.T) {
	regs, err := parseRegs(" 1080 = 75 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if regs[0].RegID != 1080 || regs[0].Value != 75 {
		t.Errorf("got %+v", regs)
	}
}

func TestParseRegs_InvalidFormat(t *testing.T) {
	_, err := parseRegs("noequals")
	if err == nil {
		t.Error("expected error for missing '='")
	}
}

func TestParseRegs_InvalidID(t *testing.T) {
	_, err := parseRegs("NOTANUMBER=75")
	if err == nil {
		t.Error("expected error for non-numeric ID")
	}
}

func TestParseRegs_InvalidValue(t *testing.T) {
	_, err := parseRegs("1080=NOTANUMBER")
	if err == nil {
		t.Error("expected error for non-numeric value")
	}
}

func TestParseRegs_Empty(t *testing.T) {
	// Empty string still yields one part with no '='
	_, err := parseRegs("")
	if err == nil {
		t.Error("expected error for empty string")
	}
}

// ── memUsagePct ───────────────────────────────────────────────────────────

func TestMemUsagePct_Normal(t *testing.T) {
	m := metrics.MemoryMetrics{UsedMB: 4096, TotalMB: 16384}
	got := memUsagePct(m)
	if got != 25 {
		t.Errorf("got %d, want 25", got)
	}
}

func TestMemUsagePct_ZeroTotal(t *testing.T) {
	m := metrics.MemoryMetrics{UsedMB: 100, TotalMB: 0}
	got := memUsagePct(m)
	if got != 0 {
		t.Errorf("got %d, want 0 for zero TotalMB", got)
	}
}

func TestMemUsagePct_Full(t *testing.T) {
	m := metrics.MemoryMetrics{UsedMB: 8192, TotalMB: 8192}
	got := memUsagePct(m)
	if got != 100 {
		t.Errorf("got %d, want 100", got)
	}
}

func TestMemUsagePct_Range(t *testing.T) {
	m := metrics.MemoryMetrics{UsedMB: 3000, TotalMB: 8192}
	got := memUsagePct(m)
	if got > 100 {
		t.Errorf("got %d, must be <= 100", got)
	}
}

// ── nextView / getCurrentFrame ────────────────────────────────────────────

func TestNextView_CyclesAllThreeViews(t *testing.T) {
	currentView = 0
	views := make(map[int]bool)
	for range 6 {
		views[currentView] = true
		nextView()
	}
	if !views[0] || !views[1] || !views[2] {
		t.Errorf("nextView must cycle through views 0, 1, and 2; only visited: %v", views)
	}
}

func TestNextView_Wraps(t *testing.T) {
	currentView = 2
	nextView()
	if currentView != 0 {
		t.Errorf("after view 2, nextView should wrap to 0, got %d", currentView)
	}
}

func TestGetCurrentFrame_AllViews(t *testing.T) {
	for v := 0; v <= 2; v++ {
		currentView = v
		frame := getCurrentFrame()
		if len(frame.Texts) == 0 {
			t.Errorf("view %d: expected at least one text block", v)
		}
	}
}

// ── cmdHelp ───────────────────────────────────────────────────────────────

func TestCmdHelp_KnownCommands(t *testing.T) {
	commands := []string{"on", "off", "brightness", "upload", "write-reg", "write-regs", "read-reg", "write-str", "show-page"}
	for _, cmd := range commands {
		h := cmdHelp(cmd)
		if !strings.Contains(h, "Command:") {
			t.Errorf("cmdHelp(%q) missing 'Command:' header", cmd)
		}
	}
}

func TestCmdHelp_UnknownCommand(t *testing.T) {
	h := cmdHelp("nonexistent")
	if !strings.Contains(h, "No help available") {
		t.Errorf("cmdHelp(unknown) should contain 'No help available', got: %q", h)
	}
}

func TestUsage_ContainsAllCommands(t *testing.T) {
	u := usage()
	for _, cmd := range []string{"on", "off", "brightness", "reboot", "upload", "write-reg", "read-reg", "write-str", "show-page"} {
		if !strings.Contains(u, cmd) {
			t.Errorf("usage() missing command %q", cmd)
		}
	}
}
