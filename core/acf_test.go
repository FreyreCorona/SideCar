package core

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeChecksum_KnownFiles(t *testing.T) {
	// Test that ComputeChecksum matches stored checksums for all production ACF files.
	dirs := []string{
		"/var/home/freyre/Development/SideCar/positivo/usr/share/minitela/resources/ACF",
		"/var/home/freyre/Development/SideCar/positivo/usr/share/minitela/resources/squashfs-root/resources/IDE_utils/ACF",
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Skipf("skipping %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".acf") {
				continue
			}
			fpath := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(fpath)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) < 8 {
				continue
			}

			expected := binary.LittleEndian.Uint32(data[4:8])
			got := ComputeChecksum(data)

			if got != expected {
				t.Errorf("%s: checksum mismatch: stored=0x%08X, computed=0x%08X", e.Name(), expected, got)
			}
		}
	}
}

func TestSetChecksum_RoundTrip(t *testing.T) {
	// Test that SetChecksum produces a valid checksum that passes ComputeChecksum.
	dirs := []string{
		"/var/home/freyre/Development/SideCar/positivo/usr/share/minitela/resources/ACF",
		"/var/home/freyre/Development/SideCar/positivo/usr/share/minitela/resources/squashfs-root/resources/IDE_utils/ACF",
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".acf") {
				continue
			}
			fpath := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(fpath)
			if err != nil {
				t.Fatal(err)
			}

			// Corrupt the checksum, then fix it.
			binary.LittleEndian.PutUint32(data[4:8], 0xDEADBEEF)
			SetChecksum(data)

			// Verify the fixed checksum matches.
			got := ComputeChecksum(data)
			stored := binary.LittleEndian.Uint32(data[4:8])
			if got != stored {
				t.Errorf("%s: after SetChecksum: computed=0x%08X != stored=0x%08X", e.Name(), got, stored)
			}

			// Verify the XOR-all-words property: XOR of all 32-bit LE words == FooterMagic.
			var xorAll uint32
			for i := 0; i+4 <= len(data); i += 4 {
				xorAll ^= binary.LittleEndian.Uint32(data[i : i+4])
			}
			if xorAll != FooterMagic {
				t.Errorf("%s: XOR of all words = 0x%08X, want 0x%08X", e.Name(), xorAll, FooterMagic)
			}
		}
	}
}

func TestBuildACF_ValidChecksum(t *testing.T) {
	// Test that BuildACF produces data with a valid checksum.
	rgb565 := make([]byte, ImageDataSize)
	// Fill with a test pattern.
	for i := range rgb565 {
		rgb565[i] = byte(i)
	}

	acf, err := BuildACF(rgb565)
	if err != nil {
		t.Fatalf("BuildACF failed: %v", err)
	}

	// Verify the checksum.
	got := ComputeChecksum(acf)
	stored := binary.LittleEndian.Uint32(acf[4:8])
	if got != stored {
		t.Errorf("BuildACF: computed=0x%08X != stored=0x%08X", got, stored)
	}

	// Verify XOR-all-words property.
	var xorAll uint32
	for i := 0; i+4 <= len(acf); i += 4 {
		xorAll ^= binary.LittleEndian.Uint32(acf[i : i+4])
	}
	if xorAll != FooterMagic {
		t.Errorf("BuildACF: XOR of all words = 0x%08X, want 0x%08X", xorAll, FooterMagic)
	}
}

func TestComputeChecksum_EmptyData(t *testing.T) {
	if got := ComputeChecksum(nil); got != 0 {
		t.Errorf("ComputeChecksum(nil) = 0x%08X, want 0", got)
	}
	if got := ComputeChecksum([]byte{}); got != 0 {
		t.Errorf("ComputeChecksum({}) = 0x%08X, want 0", got)
	}
	if got := ComputeChecksum([]byte{1, 2, 3}); got != 0 {
		t.Errorf("ComputeChecksum({1,2,3}) = 0x%08X, want 0", got)
	}
}

func TestComputeChecksum_Simple(t *testing.T) {
	// Simple case: minimal data with just magic, count, checksum=0, unknown, and footer.
	// bytes 0-3:  magic(2B) + count(2B)  → word 0
	// bytes 4-7:  checksum (skip in XOR)
	// bytes 8-11: unknown field          → word 2
	// bytes 12+:  rest of data including footer magic at the end
	//
	// For a minimal file: [magic 2B][count 2B][cksum 4B=0][unknown 4B][custom...][FooterMagic 4B]
	// checksum = XOR of word0 ^ word2 ^ ... ^ word_{n-2}

	// Construct a minimal 16-byte file:
	// Word 0 (0-3):  magic=0x0080, count=1  → 0x00010080
	// Word 1 (4-7):  checksum=0 (skipped)
	// Word 2 (8-11): unknown=0
	// Word 3 (12-15): FooterMagic=0xA55A5AA5
	// XOR of words 0 and 2: 0x00010080 ^ 0x00000000 = 0x00010080
	data := make([]byte, 16)
	binary.LittleEndian.PutUint16(data[0:2], 0x0080)   // magic
	binary.LittleEndian.PutUint16(data[2:4], 1)        // count
	binary.LittleEndian.PutUint32(data[4:8], 0)        // checksum (zeroed)
	binary.LittleEndian.PutUint32(data[8:12], 0)       // unknown
	binary.LittleEndian.PutUint32(data[12:16], FooterMagic) // footer

	got := ComputeChecksum(data)
	expected := uint32(0x00010080)
	if got != expected {
		t.Errorf("ComputeChecksum(simple) = 0x%08X, want 0x%08X", got, expected)
	}

	// After SetChecksum, XOR of all words should be FooterMagic.
	SetChecksum(data)
	var xorAll uint32
	for i := 0; i+4 <= len(data); i += 4 {
		xorAll ^= binary.LittleEndian.Uint32(data[i : i+4])
	}
	if xorAll != FooterMagic {
		t.Errorf("After SetChecksum: XOR of all words = 0x%08X, want 0x%08X", xorAll, FooterMagic)
	}
}
