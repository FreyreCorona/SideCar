package tests

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

// syncDateTimeEncoding replicates the exact encoding logic from daemon.go
// so we can test it independently without a real device.
func encodeDateVal(t time.Time) (uint32, error) {
	dateStr := fmt.Sprintf("%04d%02d%02d", t.Year(), int(t.Month()), t.Day())
	v, err := strconv.ParseUint(dateStr, 16, 32)
	return uint32(v), err
}

func encodeTimeVal(t time.Time) (uint32, error) {
	timeStr := fmt.Sprintf("%02d%02d%02d", t.Hour(), t.Minute(), t.Second())
	v, err := strconv.ParseUint(timeStr, 16, 32)
	return uint32(v), err
}

// ── Date encoding ──────────────────────────────────────────────────────────

func TestDateEncoding_MatchesJSLogic(t *testing.T) {
	// JS: dateStr = `0x${year}${month}${day}` → Number(dateStr)
	// For 2025-03-07 JS produces Number("0x20250307") = 0x20250307
	d := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)
	got, err := encodeDateVal(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x20250307)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestDateEncoding_December(t *testing.T) {
	// Month 12 must encode as 0x12, not 0x0C (arithmetic hex)
	d := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	got, err := encodeDateVal(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x20251231)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestDateEncoding_January(t *testing.T) {
	d := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := encodeDateVal(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x20240101)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestDateEncoding_NotArithmetic(t *testing.T) {
	// The old (broken) arithmetic formula: year*0x10000 + month*0x100 + day
	// For 2025-03-07: 2025*65536 + 3*256 + 7 = 0x07E90307 (WRONG)
	d := time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC)
	got, err := encodeDateVal(d)
	if err != nil {
		t.Fatal(err)
	}
	wrongArithmetic := uint32(2025)*0x10000 + uint32(3)*0x100 + uint32(7)
	if got == wrongArithmetic {
		t.Errorf("date encoding must NOT use arithmetic: got 0x%08X (old broken formula)", got)
	}
}

// ── Time encoding ──────────────────────────────────────────────────────────

func TestTimeEncoding_MatchesJSLogic(t *testing.T) {
	// For 15:30:45 JS produces Number("0x153045") = 0x00153045
	ts := time.Date(2025, 1, 1, 15, 30, 45, 0, time.UTC)
	got, err := encodeTimeVal(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x153045)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestTimeEncoding_Midnight(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := encodeTimeVal(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x000000)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestTimeEncoding_EndOfDay(t *testing.T) {
	ts := time.Date(2025, 1, 1, 23, 59, 59, 0, time.UTC)
	got, err := encodeTimeVal(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := uint32(0x235959)
	if got != want {
		t.Errorf("got 0x%08X, want 0x%08X", got, want)
	}
}

func TestTimeEncoding_NotArithmetic(t *testing.T) {
	// Old broken formula: hour*0x10000 + min*0x100 + sec
	// For 15:30:45: 15*65536 + 30*256 + 45 = 0x000F1E2D (WRONG, should be 0x153045)
	ts := time.Date(2025, 1, 1, 15, 30, 45, 0, time.UTC)
	got, err := encodeTimeVal(ts)
	if err != nil {
		t.Fatal(err)
	}
	wrongArithmetic := uint32(15)*0x10000 + uint32(30)*0x100 + uint32(45)
	if got == wrongArithmetic {
		t.Errorf("time encoding must NOT use arithmetic: got 0x%08X (old broken formula)", got)
	}
}
