package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const Baud = 115200

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("minitela", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	device := fs.String("device", "", "device path or 'auto'")
	brightness := fs.Int("brightness", -1, "brightness value (0-100)")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: minitela --device <path|auto> [--brightness N] <on|off>")
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

func connect(devicePath string) (*Device, error) {
	if devicePath == "auto" {
		return AutoConnect(Baud)
	}

	port, err := OpenSerial(devicePath, Baud)
	if err != nil {
		return nil, err
	}

	return NewDevice(port), nil
}
