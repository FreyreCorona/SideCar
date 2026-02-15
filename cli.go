package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/FreyreCorona/SideCar/core"
)

func runCLI(args []string) error {
	fs := flag.NewFlagSet("sidecar", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	device := fs.String("device", "auto", "device path or 'auto'")
	brightness := fs.Int("brightness", 100, "brightness value (0-100)")
	cmd := fs.String("command", "on", "command to send")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("usage: sidecar --device <path|auto> [--brightness N] <on|off>")
	}

	dev, err := connect(*device)
	if err != nil {
		return fmt.Errorf("unable to connect %s", err.Error())
	}
	defer dev.Close()

	dev.SetLogger(os.Stdout)
	if err := dev.Handshake(); err != nil {
		return err
	}

	switch *cmd {
	case "on":
		if err := dev.Wake(); err != nil {
			return fmt.Errorf("unable to execute 'on' command. %s", err.Error())
		}
	case "off":
		if err := dev.Sleep(); err != nil {
			return fmt.Errorf("unable to execute 'off' command. %s", err.Error())
		}
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}

	if *brightness < 0 || *brightness > 100 {
		return fmt.Errorf("brightness must be between 0 and 100")
	}

	return nil
}

func connect(devicePath string) (*core.Device, error) {
	Baud := 115200
	if devicePath == "auto" {
		return core.AutoConnect(Baud)
	}

	port, err := core.OpenSerial(devicePath, Baud)
	if err != nil {
		return nil, err
	}

	return core.NewDevice(port), nil
}
