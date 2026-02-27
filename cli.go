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
	baud := fs.Uint("baud", 11520, "baud rate")
	cmd := fs.String("cmd", "on", "command to send")
	brightness := fs.Int("brightness", 100, "brightness screen value (0-100)") //optional
	help := fs.Bool("help", false, "display command help")                     //optional

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("error : %v", err)
	}

	if *help {
		fmt.Println(printHelp(*cmd))
		return nil
	}

	if *brightness < 0 || *brightness > 100 {
		return fmt.Errorf("brightness must be between 0 and 100")
	}

	var dev *core.Device

	if *device != "auto" {
		sp, err := core.OpenSerial(*device, int(*baud))
		if err != nil {
			return fmt.Errorf("unable to stablish connection with the device :%s on baud %d ,error : %v", *device, *baud, err)
		}
		dev = core.NewDevice(sp)
	} else {
		var cErr error
		dev, cErr = core.AutoConnect(int(*baud))
		if cErr != nil {
			return fmt.Errorf("using baud %d cannot establish connection with any device, error : %v", *baud, cErr)
		}
	}

	defer dev.Close()
	dev.SetLogger(os.Stdout)
	if err := dev.Handshake(); err != nil {
		return err
	}

	if err := dev.SetBrightness(uint8(*brightness)); err != nil {
		return fmt.Errorf("unable to set brightness to %d, error : %v", *brightness, err)
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
		return fmt.Errorf("unknown command: %s", *cmd)
	}

	return nil
}

func printHelp(cmd string) string {
	switch cmd {
	case "on":
		return `Sidecar CLI

Command: on
Turns the device screen on (wake mode)

Options:
  -device string
        device path or 'auto' (default "auto")
  -baud uint
        serial baud rate (default 11520)
  -brightness int
        brightness level from 0 to 100 (default 100)

Example:
  sidecar -cmd on -brightness 80
`
	case "off":
		return `Sidecar CLI

Command: off
Turns the device screen off (sleep mode)

Options:
  -device string
        device path or 'auto' (default "auto")
  -baud uint
        serial baud rate (default 11520)

Example:
  sidecar -cmd off
`
	default:
		return `Sidecar CLI

Available commands:
  on        Wake the device
  off       Put the device to sleep

Global Options:
  -device string
        device path or 'auto' (default "auto")
  -baud uint
        serial baud rate (default 11520)
  -brightness int
        brightness level from 0 to 100 (default 100)
  -cmd string
        command to send (on/off)
  -help
        display this help

Examples:
  sidecar -cmd on
  sidecar -cmd off -device /dev/ttyUSB0
  sidecar -cmd on -brightness 50
`
	}
}
