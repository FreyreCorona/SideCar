package core

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// DisplaySize is the side length in pixels of the Minitela display.
const DisplaySize = 240

// RGB565HeaderSize is the size of the header WrapRGB565 prepends to raw
// RGB565 pixel data: [4B LE size][2B LE width][2B LE height].
const RGB565HeaderSize = 8

// ImageToRGB565 scales src to w×h using bilinear filtering and encodes the
// result as little-endian RGB565 pixels.
//
// The Minitela display stores pixels low-byte-first; writing them
// big-endian (high byte first) swaps the red and blue channels.
func ImageToRGB565(src image.Image, w, h int) ([]byte, error) {
	if src == nil {
		return nil, fmt.Errorf("ImageToRGB565: nil source image")
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("ImageToRGB565: invalid target size %dx%d", w, h)
	}

	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("ImageToRGB565: empty source image")
	}

	raw := make([]byte, w*h*2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Pixel-center aligned source coordinate (avoids edge bias).
			sx := (float64(x) + 0.5) * float64(srcW) / float64(w)
			sy := (float64(y) + 0.5) * float64(srcH) / float64(h)
			r, g, b := sampleBilinear(src, bounds, sx, sy)
			idx := (y*w + x) * 2
			binary.LittleEndian.PutUint16(raw[idx:], encodeRGB565(r, g, b))
		}
	}
	return raw, nil
}

// sampleBilinear samples an image at continuous source coordinates (sx, sy)
// using 4-point bilinear interpolation. Channel values are weighted in
// premultiplied-alpha space so semi-transparent edges blend correctly.
func sampleBilinear(src image.Image, bounds image.Rectangle, sx, sy float64) (r, g, b uint8) {
	maxX := float64(bounds.Dx() - 1)
	maxY := float64(bounds.Dy() - 1)
	if sx < 0 {
		sx = 0
	} else if sx > maxX {
		sx = maxX
	}
	if sy < 0 {
		sy = 0
	} else if sy > maxY {
		sy = maxY
	}

	x0 := int(sx)
	y0 := int(sy)
	x1, y1 := x0, y0
	if x0 < bounds.Dx()-1 {
		x1 = x0 + 1
	}
	if y0 < bounds.Dy()-1 {
		y1 = y0 + 1
	}
	dx := sx - float64(x0)
	dy := sy - float64(y0)

	minX := bounds.Min.X
	minY := bounds.Min.Y

	r00, g00, b00, a00 := src.At(minX+x0, minY+y0).RGBA()
	r10, g10, b10, a10 := src.At(minX+x1, minY+y0).RGBA()
	r01, g01, b01, a01 := src.At(minX+x0, minY+y1).RGBA()
	r11, g11, b11, a11 := src.At(minX+x1, minY+y1).RGBA()

	w00 := (1 - dx) * (1 - dy)
	w10 := dx * (1 - dy)
	w01 := (1 - dx) * dy
	w11 := dx * dy

	rp := w00*float64(r00) + w10*float64(r10) + w01*float64(r01) + w11*float64(r11)
	gp := w00*float64(g00) + w10*float64(g10) + w01*float64(g01) + w11*float64(g11)
	bp := w00*float64(b00) + w10*float64(b10) + w01*float64(b01) + w11*float64(b11)
	ap := w00*float64(a00) + w10*float64(a10) + w01*float64(a01) + w11*float64(a11)

	if ap == 0 {
		return 0, 0, 0
	}
	return uint8(rp / ap * 0xFF), uint8(gp / ap * 0xFF), uint8(bp / ap * 0xFF)
}

// encodeRGB565 packs 8-bit channels into a 16-bit RGB565 value.
func encodeRGB565(r, g, b uint8) uint16 {
	r5 := uint16(r) >> 3
	g6 := uint16(g) >> 2
	b5 := uint16(b) >> 3
	return (r5 << 11) | (g6 << 5) | b5
}

// DecodeRGB565 unpacks little-endian RGB565 pixels into an RGBA image.
// Used by tests to verify encoding round-trips.
func DecodeRGB565(data []byte, w, h int) (*image.RGBA, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("DecodeRGB565: invalid size %dx%d", w, h)
	}
	if len(data) != w*h*2 {
		return nil, fmt.Errorf("DecodeRGB565: expected %d bytes for %dx%d, got %d", w*h*2, w, h, len(data))
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		rgb := binary.LittleEndian.Uint16(data[i*2:])
		r5 := (rgb >> 11) & 0x1F
		g6 := (rgb >> 5) & 0x3F
		b5 := rgb & 0x1F
		img.SetRGBA(i%w, i/w, color.RGBA{
			R: uint8(r5)<<3 | uint8(r5)>>2,
			G: uint8(g6)<<2 | uint8(g6)>>4,
			B: uint8(b5)<<3 | uint8(b5)>>2,
			A: 255,
		})
	}
	return img, nil
}

// WrapRGB565 prepends the 8-byte little-endian header
// (size, width, height) to raw RGB565 pixel data.
func WrapRGB565(rgb []byte, w, h int) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("WrapRGB565: invalid size %dx%d", w, h)
	}
	if len(rgb) != w*h*2 {
		return nil, fmt.Errorf("WrapRGB565: expected %d bytes for %dx%d, got %d", w*h*2, w, h, len(rgb))
	}

	out := make([]byte, RGB565HeaderSize+len(rgb))
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(rgb)))
	binary.LittleEndian.PutUint16(out[4:6], uint16(w))
	binary.LittleEndian.PutUint16(out[6:8], uint16(h))
	copy(out[RGB565HeaderSize:], rgb)
	return out, nil
}

// UnwrapRGB565 parses the 8-byte little-endian header and returns the raw
// RGB565 payload with its dimensions.
func UnwrapRGB565(data []byte) (rgb []byte, w, h int, err error) {
	if len(data) < RGB565HeaderSize {
		return nil, 0, 0, fmt.Errorf("UnwrapRGB565: need at least %d bytes, got %d", RGB565HeaderSize, len(data))
	}
	size := binary.LittleEndian.Uint32(data[0:4])
	w = int(binary.LittleEndian.Uint16(data[4:6]))
	h = int(binary.LittleEndian.Uint16(data[6:8]))
	if w <= 0 || h <= 0 {
		return nil, 0, 0, fmt.Errorf("UnwrapRGB565: invalid dimensions %dx%d", w, h)
	}
	if int(size) != w*h*2 {
		return nil, 0, 0, fmt.Errorf("UnwrapRGB565: size field %d does not match %dx%d pixels", size, w, h)
	}
	if int(size) != len(data)-RGB565HeaderSize {
		return nil, 0, 0, fmt.Errorf("UnwrapRGB565: size field %d does not match payload of %d bytes", size, len(data)-RGB565HeaderSize)
	}
	return data[RGB565HeaderSize:], w, h, nil
}

// ImageToACF converts an image to a complete 240×240 ACF project file,
// ready to upload as a texture. Images of any size are bilinear-scaled.
func ImageToACF(src image.Image) ([]byte, error) {
	rgb, err := ImageToRGB565(src, DisplaySize, DisplaySize)
	if err != nil {
		return nil, err
	}
	return BuildACF(rgb)
}
