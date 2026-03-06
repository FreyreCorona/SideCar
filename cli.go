package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/FreyreCorona/SideCar/core"
)

func runCLI(args []string) error {
	fs := flag.NewFlagSet("sidecar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Global flags
	device := fs.String("device", "auto", "device path or 'auto'")
	baud := fs.Uint("baud", 115200, "baud rate")
	cmd := fs.String("cmd", "", "command to run (required)")
	help := fs.Bool("help", false, "display command help")

	// Per-command flags
	brightness := fs.Int("brightness", 100, "brightness level 0-255 [cmd: on, brightness]")
	filePath := fs.String("file", "", "path to file to upload [cmd: upload]")
	fileType := fs.String("type", "texture", "file type: texture, texture_gif, cpu1, bootloader, ... [cmd: upload]")
	regID := fs.Uint("reg", 0, "register ID (uint16) [cmd: write-reg, read-reg]")
	regVal := fs.Uint("val", 0, "register value (uint32) [cmd: write-reg]")
	regStr := fs.String("str", "", "string value [cmd: write-str]")
	pageID := fs.Int("page", 1, "page number to display [cmd: show-page]")
	regs := fs.String("regs", "", "registers as \"ID1=VAL1,ID2=VAL2,...\" [cmd: write-regs]")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("error parsing flags: %v\n\n%s", err, usage())
	}

	if *help || *cmd == "" {
		if *cmd != "" {
			fmt.Println(cmdHelp(*cmd))
		} else {
			fmt.Println(usage())
		}
		return nil
	}

	// ── Connection ────────────────────────────────────────────────────────────

	dev, err := connect(*device, int(*baud))
	if err != nil {
		return err
	}
	defer dev.Close()
	dev.SetLogger(os.Stdout)

	// ── Dispatch ──────────────────────────────────────────────────────────────

	switch *cmd {

	case "on":
		if *brightness < 0 || *brightness > 255 {
			return fmt.Errorf("-brightness must be between 0 and 255")
		}
		return dev.SetBrightness(uint8(*brightness))

	case "off":
		return dev.Sleep()

	case "brightness":
		if *brightness < 0 || *brightness > 255 {
			return fmt.Errorf("-brightness must be between 0 and 255")
		}
		return dev.SetBrightness(uint8(*brightness))

	case "reboot":
		return dev.Reboot()

	case "upload":
		if *filePath == "" {
			return fmt.Errorf("upload requires -file <path>")
		}

		data, err := os.ReadFile(*filePath)
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", *filePath, err)
		}

		cfg := core.UploadConfig{
			OnProgress: func(p core.UploadProgress) {
				fmt.Printf("\r  uploading... %d%%", p.Percent)
				if p.Percent == 100 {
					fmt.Println()
				}
			},
		}

		if err := dev.UploadFile(data, core.FileType(*fileType), cfg); err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}

		fmt.Printf("✓ file uploaded (%s)\n", *fileType)
		return nil

	case "write-reg":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg must be a valid uint16")
		}
		result, err := dev.WriteNumRegisters([]core.NumRegister{
			{RegID: uint16(*regID), Value: uint32(*regVal)},
		})
		if err != nil {
			return fmt.Errorf("write-reg failed: %w", err)
		}
		for id, val := range result {
			fmt.Printf("✓ reg[0x%04X] = %d\n", id, val)
		}
		return nil

	case "read-reg":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg must be a valid uint16")
		}
		result, err := dev.ReadNumRegisters([]uint16{uint16(*regID)})
		if err != nil {
			return fmt.Errorf("read-reg failed: %w", err)
		}
		for id, val := range result {
			fmt.Printf("reg[0x%04X] = %d (0x%08X)\n", id, val, val)
		}
		return nil

	case "write-str":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg must be a valid uint16")
		}
		if *regStr == "" {
			return fmt.Errorf("write-str requires -str <value>")
		}
		data := []byte(*regStr)
		_, err := dev.WriteStringRegister(uint16(*regID), data)
		if err != nil {
			return fmt.Errorf("write-str failed: %w", err)
		}
		fmt.Printf("✓ reg[0x%04X] = %q\n", *regID, *regStr)
		return nil

	case "read-str":
		if *regID > 0xFFFF {
			return fmt.Errorf("-reg must be a valid uint16")
		}
		data, err := dev.ReadStringRegister(uint16(*regID), 64)
		if err != nil {
			return fmt.Errorf("read-str failed: %w", err)
		}
		fmt.Printf("reg[0x%04X] = %q (%s)\n", *regID, string(data), hex.EncodeToString(data))
		return nil

	case "show-page":
		if *pageID < 1 {
			return fmt.Errorf("-page must be >= 1")
		}
		result, err := dev.WriteNumRegisters([]core.NumRegister{
			{RegID: core.RegCurrentPage, Value: uint32(*pageID)},
		})
		if err != nil {
			return fmt.Errorf("show-page failed: %w", err)
		}
		for _, val := range result {
			fmt.Printf("✓ active page = %d\n", val)
		}
		return nil

	case "write-regs":
		if *regs == "" {
			return fmt.Errorf("write-regs requires -regs \"ID1=VAL1,ID2=VAL2,...\"")
		}
		parsedRegs, err := parseRegs(*regs)
		if err != nil {
			return err
		}
		result, err := dev.WriteNumRegisters(parsedRegs)
		if err != nil {
			return fmt.Errorf("write-regs failed: %w", err)
		}
		for id, val := range result {
			fmt.Printf("✓ reg[0x%04X] = %d\n", id, val)
		}
		return nil

	default:
		return fmt.Errorf("unknown command: %q\n\n%s", *cmd, usage())
	}
}

// pageRegID and regBrightness use the constants defined in core/tags.go.
// '当前页面序号' → RegCurrentPage = 2  (not 1, confirmed in tagUtils.js)
// '背光'         → RegBrightness  = 7

// ── Helpers ────────────────────────────────────────────────────────────────────

func connect(device string, baud int) (*core.Device, error) {
	if device != "auto" {
		sp, err := core.OpenSerial(device, baud)
		if err != nil {
			return nil, fmt.Errorf("cannot open %s at %d baud: %w", device, baud, err)
		}
		dev := core.NewDevice(sp)
		if _, err := dev.Handshake(); err != nil {
			dev.Close()
			return nil, fmt.Errorf("handshake failed on %s: %w", device, err)
		}
		return dev, nil
	}

	dev, err := core.AutoConnect(baud)
	if err != nil {
		return nil, fmt.Errorf("no compatible device found at %d baud: %w", baud, err)
	}
	return dev, nil
}

func parseRegs(input string) ([]core.NumRegister, error) {
	parts := strings.Split(input, ",")
	regs := make([]core.NumRegister, 0, len(parts))
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid format %q, use ID=VALUE", p)
		}
		id, err := strconv.ParseUint(strings.TrimSpace(kv[0]), 0, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid regID %q: %w", kv[0], err)
		}
		val, err := strconv.ParseUint(strings.TrimSpace(kv[1]), 0, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q: %w", kv[1], err)
		}
		regs = append(regs, core.NumRegister{RegID: uint16(id), Value: uint32(val)})
	}
	return regs, nil
}

// ── Help strings ──────────────────────────────────────────────────────────────

func usage() string {
	return `Sidecar CLI — control a Minitela over serial

Usage:
  sidecar -cmd <command> [options]

Commands:
  on            Turn the screen on (default brightness 100)
  off           Turn the screen off
  brightness    Adjust brightness without changing screen state
  reboot        Reboot the device
  upload        Upload a firmware or texture file
  write-reg     Write a numeric register
  write-regs    Write multiple registers in a single batch
  read-reg      Read a numeric register
  write-str     Write a string register
  read-str      Read a string register
  show-page     Change the active page on the screen

Global options:
  -device string   serial port path or 'auto' (default "auto")
  -baud uint       baud rate (default 115200)
  -help            show help; combined with -cmd shows command-specific help

Examples:
  sidecar -cmd on
  sidecar -cmd on -brightness 80
  sidecar -cmd off -device /dev/ttyUSB0
  sidecar -cmd upload -file Texture.acf -type texture
  sidecar -cmd write-reg -reg 0x0001 -val 3
  sidecar -cmd write-str -reg 0x0010 -str "Hello world"
  sidecar -cmd show-page -page 2
  sidecar -cmd on -help
`
}

func cmdHelp(cmd string) string {
	helps := map[string]string{
		"on": `Command: on
Turn the screen on by setting the brightness.

Options:
  -brightness int   brightness level 0–255 (default 100)

Examples:
  sidecar -cmd on
  sidecar -cmd on -brightness 200
`,
		"off": `Command: off
Turn the screen off (brightness = 0).

Example:
  sidecar -cmd off
`,
		"brightness": `Command: brightness
Adjust brightness without changing the screen state.

Options:
  -brightness int   brightness level 0–255 (required)

Example:
  sidecar -cmd brightness -brightness 150
`,
		"upload": `Command: upload
Upload a binary file (.acf) to the device. Supports automatic resume.

Workflow for images and GIFs:
  1. Generate the .acf with AHMISimGenDemo.exe (included in IDE_utils/Gen/)
       AHMISimGenDemo.exe -f file.zip -m 2 -c 0 -e -o ./ACF
  2. Upload the generated file:
       sidecar -cmd upload -file Texture.acf  -type texture
       sidecar -cmd upload -file GIF_0.acf    -type texture_gif

Options:
  -file string   path to the .acf file (required)
  -type string   destination on the device (default "texture"):
                   texture       → 0x08100000  main image/texture
                   texture_gif   → 0x08500000  animated GIF
                   cpu1          → main firmware
                   cpu0_config   → CPU0 configuration
                   bootloader    → bootloader (use with caution)

Examples:
  sidecar -cmd upload -file Texture.acf -type texture
  sidecar -cmd upload -file GIF_0.acf -type texture_gif
`,
		"write-regs": `Command: write-regs
Write multiple numeric registers in a single batch (more efficient than repeated write-reg).

Options:
  -regs string   register list in the format "ID1=VAL1,ID2=VAL2,..." (required)
                 IDs can be decimal or hex (0x...)

Examples:
  sidecar -cmd write-regs -regs "1080=75,1081=40,1082=90"
  sidecar -cmd write-regs -regs "0x0438=75,0x0439=40"
`,
		"write-reg": `Command: write-reg
Write a numeric value (uint32) to a device register.

Options:
  -reg uint   register ID in hex or decimal (required)
  -val uint   value to write (default 0)

Examples:
  sidecar -cmd write-reg -reg 0x0001 -val 3
  sidecar -cmd write-reg -reg 7 -val 100
`,
		"read-reg": `Command: read-reg
Read the numeric value of a register.

Options:
  -reg uint   register ID (required)

Example:
  sidecar -cmd read-reg -reg 0x0001
`,
		"write-str": `Command: write-str
Write a string value to a device register.

Options:
  -reg uint      register ID (required)
  -str string    text to write (required)

Example:
  sidecar -cmd write-str -reg 0x0010 -str "22°C"
`,
		"show-page": `Command: show-page
Change the active page on the screen.

Options:
  -page int   page number (default 1)

Example:
  sidecar -cmd show-page -page 3
`,
	}

	if h, ok := helps[cmd]; ok {
		return h
	}
	return fmt.Sprintf("No help available for %q\n\n%s", cmd, usage())
}
