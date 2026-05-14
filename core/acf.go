package core

import (
	"encoding/binary"
	"fmt"
	"strings"
)

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
// Reference: pre-built .acf files at positivo/usr/share/minitela/resources/ACF/
//
// Layout (16 KB header at offset 0):
//
//	0x0000: [2]byte  magic           0x0080 (texture) or 0x0000 (config)
//	0x0002: uint16   count           big-endian, number of resources (?)
//	0x0004: uint32   checksum        4 bytes, purpose unknown
//	0x0008: uint32   compressedSize  big-endian, size of compressed project data
//	0x000C: [24]byte projectID       ASCII hex string e.g. "67004c7703ad6966e4fe0d13"
//	0x0024: [44]byte _padding
//	0x0050: [32]byte versionA        e.g. "1.10.19_build_2024.09.20_GIFTest"
//	0x0070: [32]byte versionB        e.g. "1.10.19_build_2025.1.9.1.17.49"
//	0x0094: [12]byte tagCPU0Version  "CPU0VERSION\0"
//	0x00B0: [32]byte cpu0Version     version string
//	0x00D0: [12]byte tagCPU0DeviceID "CPU0DEVICEID\0"
//	0x00FC: uint16   deviceIDLen     length of device ID data
//	0x0100: [8]byte  deviceID        8 bytes of device identifier
//	0x0108: uint16   screenWidth     typically 0xF000 = 240 (scaled?)
//	0x010A: uint16   screenHeight    typically 0xF000 = 240
//	0x010C: [16]byte _unknown
//	0x011C: [36]byte deviceInfo      e.g. "IDE001.4HWX0104.00EM0304.01NOR256D36"
//	0x0140: [...]    _zeros          rest of 16KB header is zero-filled
//
// After the header, the compressed/encoded project data starts at offset 0x4000.
// Resource blocks (64 KB each) may follow the project data.
type ACFHeader struct {
	Magic          ACFMagic
	Count          uint16
	Checksum       uint32
	CompressedSize uint32
	ProjectID      [24]byte
	VersionA       [32]byte
	VersionB       [32]byte
	CPU0Version    string
	DeviceInfo     string
	ScreenWidth    uint16
	ScreenHeight   uint16
}

// ParseACFHeader parses an ACF header from the first 16 KB of data.
func ParseACFHeader(data []byte) (*ACFHeader, error) {
	if len(data) < ACFHeaderSize {
		return nil, fmt.Errorf("ACF header requires %d bytes, got %d", ACFHeaderSize, len(data))
	}

	h := &ACFHeader{
		Magic:          ACFMagic(binary.BigEndian.Uint16(data[0:2])),
		Count:          binary.BigEndian.Uint16(data[2:4]),
		Checksum:       binary.BigEndian.Uint32(data[4:8]),
		CompressedSize: binary.BigEndian.Uint32(data[8:12]),
		ScreenWidth:    binary.BigEndian.Uint16(data[0x108:0x10A]),
		ScreenHeight:   binary.BigEndian.Uint16(data[0x10A:0x10C]),
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
	fmt.Fprintf(&b, "  compressedSize=%d (0x%X)\n", h.CompressedSize, h.CompressedSize)
	fmt.Fprintf(&b, "  projectID=%s\n", nullTrim(h.ProjectID[:]))
	fmt.Fprintf(&b, "  versionA=%s\n", nullTrim(h.VersionA[:]))
	fmt.Fprintf(&b, "  versionB=%s\n", nullTrim(h.VersionB[:]))
	fmt.Fprintf(&b, "  cpu0Version=%s\n", h.CPU0Version)
	fmt.Fprintf(&b, "  deviceInfo=%s\n", h.DeviceInfo)
	fmt.Fprintf(&b, "  screen=%dx%d\n", h.ScreenWidth, h.ScreenHeight)
	return b.String()
}

// ProjectDataOffset returns the offset where the compressed project data begins.
func (h *ACFHeader) ProjectDataOffset() int {
	return ACFHeaderSize
}

// TotalSize estimates the total file size given the number of resource blocks.
// fileSize = headerSize + projectData + resourceBlocks * ResourceBlockSize
func (h *ACFHeader) TotalSize(numBlocks int) int64 {
	return int64(ACFHeaderSize) + int64(h.CompressedSize) + int64(numBlocks)*ResourceBlockSize
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
