package core

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// solidImage returns a w×h image filled with the given color.
func solidImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestImageToRGB565_ByteOrder(t *testing.T) {
	// The device stores RGB565 little-endian (low byte first).
	// Red (0xF800) → [0x00, 0xF8], Green (0x07E0) → [0xE0, 0x07], Blue (0x001F) → [0x1F, 0x00].
	cases := []struct {
		name string
		rgb  color.RGBA
		want []byte
	}{
		{"red", color.RGBA{255, 0, 0, 255}, []byte{0x00, 0xF8}},
		{"green", color.RGBA{0, 255, 0, 255}, []byte{0xE0, 0x07}},
		{"blue", color.RGBA{0, 0, 255, 255}, []byte{0x1F, 0x00}},
		{"white", color.RGBA{255, 255, 255, 255}, []byte{0xFF, 0xFF}},
		{"black", color.RGBA{0, 0, 0, 255}, []byte{0x00, 0x00}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ImageToRGB565(solidImage(1, 1, tc.rgb), 1, 1)
			if err != nil {
				t.Fatalf("ImageToRGB565: %v", err)
			}
			if len(got) != 2 {
				t.Fatalf("expected 2 bytes, got %d", len(got))
			}
			if got[0] != tc.want[0] || got[1] != tc.want[1] {
				t.Errorf("bytes = [%02X %02X], want [%02X %02X] (little-endian RGB565)", got[0], got[1], tc.want[0], tc.want[1])
			}
		})
	}
}

func TestImageToRGB565_SolidScalesAndMatchesDecode(t *testing.T) {
	// A solid 1920×1080 image must scale down to 240×240 and round-trip
	// through DecodeRGB565 to approximately the same color.
	c := color.RGBA{200, 100, 50, 255}
	raw, err := ImageToRGB565(solidImage(1920, 1080, c), DisplaySize, DisplaySize)
	if err != nil {
		t.Fatalf("ImageToRGB565: %v", err)
	}
	if len(raw) != ImageDataSize {
		t.Fatalf("expected %d bytes, got %d", ImageDataSize, len(raw))
	}

	img, err := DecodeRGB565(raw, DisplaySize, DisplaySize)
	if err != nil {
		t.Fatalf("DecodeRGB565: %v", err)
	}
	got := img.RGBAAt(0, 0)
	if got.A != 255 {
		t.Errorf("alpha = %d, want 255", got.A)
	}
	// Allow ±8 quantization error per channel (5/6-bit rounding).
	if diff(got.R, c.R) > 8 || diff(got.G, c.G) > 8 || diff(got.B, c.B) > 8 {
		t.Errorf("center pixel = %v, want ~%v", got, c)
	}
}

func TestImageToRGB565_SquareHDSource(t *testing.T) {
	// 2048×2048 source must also reduce cleanly to 240×240.
	raw, err := ImageToRGB565(solidImage(2048, 2048, color.RGBA{1, 2, 3, 255}), DisplaySize, DisplaySize)
	if err != nil {
		t.Fatalf("ImageToRGB565: %v", err)
	}
	if len(raw) != ImageDataSize {
		t.Fatalf("expected %d bytes, got %d", ImageDataSize, len(raw))
	}
}

func TestImageToRGB565_Errors(t *testing.T) {
	if _, err := ImageToRGB565(nil, 10, 10); err == nil {
		t.Error("expected error for nil image")
	}
	if _, err := ImageToRGB565(solidImage(10, 10, color.RGBA{}), 0, 10); err == nil {
		t.Error("expected error for zero width")
	}
	if _, err := ImageToRGB565(solidImage(10, 10, color.RGBA{}), 10, -1); err == nil {
		t.Error("expected error for negative height")
	}
}

func TestWrapUnwrapRGB565(t *testing.T) {
	w, h := 240, 240
	rgb := make([]byte, w*h*2)
	for i := range rgb {
		rgb[i] = byte(i)
	}

	wrapped, err := WrapRGB565(rgb, w, h)
	if err != nil {
		t.Fatalf("WrapRGB565: %v", err)
	}
	if len(wrapped) != RGB565HeaderSize+len(rgb) {
		t.Fatalf("wrapped length = %d, want %d", len(wrapped), RGB565HeaderSize+len(rgb))
	}
	// Header fields must be little-endian.
	if got := binary.LittleEndian.Uint32(wrapped[0:4]); got != uint32(len(rgb)) {
		t.Errorf("size field = %d, want %d", got, len(rgb))
	}
	if got := binary.LittleEndian.Uint16(wrapped[4:6]); int(got) != w {
		t.Errorf("width field = %d, want %d", got, w)
	}
	if got := binary.LittleEndian.Uint16(wrapped[6:8]); int(got) != h {
		t.Errorf("height field = %d, want %d", got, h)
	}

	gotRGB, gotW, gotH, err := UnwrapRGB565(wrapped)
	if err != nil {
		t.Fatalf("UnwrapRGB565: %v", err)
	}
	if gotW != w || gotH != h {
		t.Errorf("dimensions = %dx%d, want %dx%d", gotW, gotH, w, h)
	}
	if len(gotRGB) != len(rgb) {
		t.Fatalf("payload length = %d, want %d", len(gotRGB), len(rgb))
	}
	for i := range rgb {
		if gotRGB[i] != rgb[i] {
			t.Errorf("payload mismatch at byte %d", i)
			break
		}
	}
}

func TestWrapRGB565_Errors(t *testing.T) {
	if _, err := WrapRGB565([]byte{1, 2, 3}, 240, 240); err == nil {
		t.Error("expected error for mismatched payload size")
	}
	if _, err := WrapRGB565(nil, 0, 0); err == nil {
		t.Error("expected error for zero dimensions")
	}
}

func TestUnwrapRGB565_Errors(t *testing.T) {
	if _, _, _, err := UnwrapRGB565([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for too-short data")
	}
	// Header says 240×240 (115200 bytes) but only 10 bytes follow.
	bad := make([]byte, RGB565HeaderSize+10)
	binary.LittleEndian.PutUint32(bad[0:4], uint32(DisplaySize*DisplaySize*2))
	binary.LittleEndian.PutUint16(bad[4:6], DisplaySize)
	binary.LittleEndian.PutUint16(bad[6:8], DisplaySize)
	if _, _, _, err := UnwrapRGB565(bad); err == nil {
		t.Error("expected error for size/payload mismatch")
	}
}

func TestImageToACF_ValidChecksum(t *testing.T) {
	acf, err := ImageToACF(solidImage(800, 600, color.RGBA{30, 60, 90, 255}))
	if err != nil {
		t.Fatalf("ImageToACF: %v", err)
	}
	if len(acf) != len(acfTemplateData) {
		t.Errorf("ACF length = %d, want %d", len(acf), len(acfTemplateData))
	}

	// Checksum must validate.
	got := ComputeChecksum(acf)
	stored := binary.LittleEndian.Uint32(acf[4:8])
	if got != stored {
		t.Errorf("checksum: computed=0x%08X != stored=0x%08X", got, stored)
	}
}

func TestParseACFHeader_EmbeddedTemplate(t *testing.T) {
	// The embedded template is the ground truth for the header layout:
	// the display is 240×240 and dims live at 0x104/0x106, not 0x108/0x10A.
	h, err := ParseACFHeader(acfTemplateData)
	if err != nil {
		t.Fatalf("ParseACFHeader: %v", err)
	}
	if h.ScreenWidth != 240 {
		t.Errorf("ScreenWidth = %d, want 240", h.ScreenWidth)
	}
	if h.ScreenHeight != 240 {
		t.Errorf("ScreenHeight = %d, want 240", h.ScreenHeight)
	}
	if h.Count != 12 {
		t.Errorf("Count = %d, want 12", h.Count)
	}
}

func diff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
