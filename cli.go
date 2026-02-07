package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/FreyreCorona/SideCar/core"
)

const Baud = 115200

func runCLI(args []string) error {
	fs := flag.NewFlagSet("sidecar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	device := fs.String("device", "", "device path or 'auto'")
	brightness := fs.Int("brightness", -1, "brightness value (0-100)")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: sidecar --device <path|auto> [--brightness N] <on|off>")
	}

	if *device == "" {
		return fmt.Errorf("missing --device")
	}

	rest := fs.Args()
	if len(rest) < 1 {
		return fmt.Errorf("missing command (on|off)")
	}

	command := rest[0]

	dev, err := connect(*device)
	if err != nil {
		return err
	}
	defer dev.Close()

	if err := dev.Handshake(); err != nil {
		return err
	}

	switch command {
	case "on":
		if err := dev.Wake(); err != nil {
			return err
		}
	case "off":
		if err := dev.Sleep(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown command: %s", command)
	}

	if *brightness != -1 {
		if *brightness < 0 || *brightness > 100 {
			return fmt.Errorf("brightness must be between 0 and 100")
		}
		return dev.SetBrightness(uint8(*brightness))
	}

	return nil
}

func connect(devicePath string) (*core.Device, error) {
	if devicePath == "auto" {
		return core.AutoConnect(Baud)
	}

	port, err := core.OpenSerial(devicePath, Baud)
	if err != nil {
		return nil, err
	}

	return core.NewDevice(port), nil
}
