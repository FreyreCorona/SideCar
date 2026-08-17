package core

import (
	"encoding/binary"
	"fmt"
	"strings"

	_ "embed"
)

//go:embed Texture.acf
var acfTemplateData []byte

// ACFHeaderSize is the size of the project header in an ACF file.
const ACFHeaderSize = 0x4000 // 16 KB

// ResourceBlockSize is the size of each resource block appended after the project data.
const ResourceBlockSize = 0x10000 // 64 KB

// ACFMagic is the 2-byte magic that starts an ACF project header.
// Texture ACFs use 0x0080, ConfigData uses 0x0000.
type ACFMagic uint16

const (
	ACFMagicTexture    ACFMagic = 0x0080
	ACFMagicConfigData ACFMagic = 0x0000
)

func (m ACFMagic) String() string {
	switch m {
	case ACFMagicTexture:
		return "texture"
	case ACFMagicConfigData:
		return "config"
	default:
		return fmt.Sprintf("unknown(0x%04X)", uint16(m))
	}
}

// ACFHeader represents the parsed header of an ACF project file.
// Reference: pre-built .acf files and core/acf.go analysis.
//
// Layout (16 KB header at offset 0, little-endian multi-byte fields):
//
//	0x0000: [2]byte  magic           0x0080 (texture) or 0x0000 (config)
//	0x0002: uint16   count           little-endian, number of 64KB resource blocks
//	0x0004: uint32   checksum        XOR of all 32-bit LE words except this field and the last word (FooterMagic)
//	0x0008: uint32   unknown         purpose unknown (not a simple payload size)
//	0x000C: [24]byte projectID       ASCII hex string e.g. "67004c7703ad6966e4fe0d13"
//	0x0024: [44]byte _padding
//	0x0050: [32]byte versionA        e.g. "1.10.19_build_2024.09.20_GIFTest"
//	0x0070: [32]byte versionB        e.g. "1.10.19_build_2025.1.9.1.17.49"
//	0x0094: [12]byte tagCPU0Version  "CPU0VERSION\0"
//	0x00B0: [32]byte cpu0Version     version string
//	0x00D0: [12]byte tagCPU0DeviceID "CPU0DEVICEID\0"
//	0x00FA: uint16   deviceIDLen     length of device ID data (observed 3)
//	0x00FC: [4]byte  _padding        zero-filled
//	0x0100: [4]byte  deviceID        device identifier bytes
//	0x0104: uint16   screenWidth     observed 0x00F0 = 240 (LE)
//	0x0106: uint16   screenHeight    observed 0x00F0 = 240 (LE)
//	0x0108: uint16   _unknown        observed 0x0010
//	0x010A: uint16   _unknown        observed 0x0020
//	0x010C: [16]byte _unknown
//	0x011C: [36]byte deviceInfo      e.g. "IDE001.4HWX0104.00EM0304.01NOR256D36"
//	0x0140: [...]    _zeros          rest of 16KB header is zero-filled
//
// After the header, the file layout is:
//
//	[header (16KB)]
//	[count × 64KB resource blocks]
//	[project data (variable, ~49KB for textures)]
type ACFHeader struct {
	Magic        ACFMagic
	Count        uint16
	Checksum     uint32
	Unknown      uint32 // purpose unclear (not totalPayload as previously thought)
	ProjectID    [24]byte
	VersionA     [32]byte
	VersionB     [32]byte
	CPU0Version  string
	DeviceInfo   string
	ScreenWidth  uint16
	ScreenHeight uint16
}

// ParseACFHeader parses an ACF header from the first 16 KB of data.
func ParseACFHeader(data []byte) (*ACFHeader, error) {
	if len(data) < ACFHeaderSize {
		return nil, fmt.Errorf("ACF header requires %d bytes, got %d", ACFHeaderSize, len(data))
	}

	h := &ACFHeader{
		Magic:       ACFMagic(binary.LittleEndian.Uint16(data[0:2])),
		Count:       binary.LittleEndian.Uint16(data[2:4]),
		Checksum:    binary.LittleEndian.Uint32(data[4:8]),
		Unknown:     binary.LittleEndian.Uint32(data[8:12]),
		ScreenWidth:  binary.LittleEndian.Uint16(data[0x104:0x106]),
		ScreenHeight: binary.LittleEndian.Uint16(data[0x106:0x108]),
	}
	copy(h.ProjectID[:], data[0x0C:0x24])
	copy(h.VersionA[:], data[0x50:0x70])
	copy(h.VersionB[:], data[0x70:0x90])

	h.CPU0Version = cstring(data[0xB0:0xD0])
	h.DeviceInfo = cstring(data[0x11C:0x140])

	return h, nil
}

// String returns a human-readable summary of the header.
func (h *ACFHeader) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "ACF magic=%s count=%d checksum=0x%08X\n", h.Magic, h.Count, h.Checksum)
	fmt.Fprintf(&b, "  unknown=0x%08X\n", h.Unknown)
	fmt.Fprintf(&b, "  projectID=%s\n", nullTrim(h.ProjectID[:]))
	fmt.Fprintf(&b, "  versionA=%s\n", nullTrim(h.VersionA[:]))
	fmt.Fprintf(&b, "  versionB=%s\n", nullTrim(h.VersionB[:]))
	fmt.Fprintf(&b, "  cpu0Version=%s\n", h.CPU0Version)
	fmt.Fprintf(&b, "  deviceInfo=%s\n", h.DeviceInfo)
	fmt.Fprintf(&b, "  screen=%dx%d\n", h.ScreenWidth, h.ScreenHeight)
	return b.String()
}

// ProjectDataOffset returns the offset where the project data begins
// (after the header and all resource blocks).
func (h *ACFHeader) ProjectDataOffset() int64 {
	return int64(ACFHeaderSize) + int64(h.Count)*ResourceBlockSize
}

// ImageDataOffset returns the file offset where the main image pixel data begins.
// In the ACF template, the image spans from block 10, offset 0x37B0 through
// block 11. The first 115200 bytes are 240×240 RGB565 LE pixel data.
const ImageDataOffset = 0x4000 + 10*ResourceBlockSize + 0x37B0

// ImageDataSize is the size of the raw RGB565 pixel data for a 240×240 image.
const ImageDataSize = 240 * 240 * 2 // 115200

// FooterMagic is the 4-byte magic value at the end of every ACF file.
const FooterMagic = 0xA55A5AA5

// FooterSize is the size of the ACF footer: [24 zero bytes][4B word][4B magic].
const FooterSize = 32

// ComputeChecksum computes the ACF checksum from completed file data.
//
// The algorithm is:
//  1. Zero the checksum field at bytes 4-7.
//  2. XOR all 32-bit little-endian words in the file EXCEPT:
//     a) The checksum field itself (bytes 4-7, already zeroed)
//     b) The very last word (always FooterMagic 0xA55A5AA5)
//  3. The resulting 32-bit value is the checksum.
//
// When the correct checksum is placed at bytes 4-7, the XOR of all 32-bit
// words in the file equals FooterMagic.
func ComputeChecksum(data []byte) uint32 {
	if len(data) < 8 {
		return 0
	}
	var xorSum uint32
	n := len(data)
	// Iterate over each 4-byte word, skipping the checksum field (offset 4)
	// and the last word (the FooterMagic marker at n-4).
	for i := 0; i+4 <= n; i += 4 {
		if i == 4 {
			// Skip the checksum field itself.
			continue
		}
		if i == n-4 {
			// Skip the last word (FooterMagic).
			continue
		}
		xorSum ^= binary.LittleEndian.Uint32(data[i : i+4])
	}
	return xorSum
}

// SetChecksum computes and sets the checksum at bytes 4-7 in-place.
func SetChecksum(data []byte) {
	// Zero the checksum field before computing.
	binary.LittleEndian.PutUint32(data[4:8], 0)
	cksum := ComputeChecksum(data)
	binary.LittleEndian.PutUint32(data[4:8], cksum)
}

// BuildACF creates a complete ACF project file from raw RGB565 pixel data
// (240×240, little-endian). It uses the embedded Texture.acf as a template
// and replaces the pixel data in the appropriate resource blocks, then
// recomputes the checksum.
func BuildACF(rgb565 []byte) ([]byte, error) {
	if len(rgb565) != ImageDataSize {
		return nil, fmt.Errorf("BuildACF: expected %d bytes of RGB565 data (240×240), got %d", ImageDataSize, len(rgb565))
	}

	acf := make([]byte, len(acfTemplateData))
	copy(acf, acfTemplateData)

	// Replace pixel data at the image offset.
	copy(acf[ImageDataOffset:], rgb565)

	// Recompute and set the correct checksum.
	SetChecksum(acf)

	return acf, nil
}

// cstring extracts a null-terminated string from a fixed-size byte slice.
func cstring(b []byte) string {
	i := 0
	for i < len(b) && b[i] != 0 {
		i++
	}
	return string(b[:i])
}

// nullTrim returns the content up to the first null byte.
func nullTrim(b []byte) string {
	return cstring(b)
}
