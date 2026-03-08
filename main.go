package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main selects the runtime mode using the -mode flag.
//
// Usage:
//
//	sidecar                    → UI (default)
//	sidecar -mode ui           → UI webview
//	sidecar -mode daemon       → daemon
//	sidecar -mode daemon -interval 3s
//	sidecar -mode cli -cmd on  → CLI
func main() {
	mode := flag.String("mode", "ui", "runtime mode: ui | daemon | cli")

	flag.Parse()

	var err error
	switch *mode {
	case "daemon":
		err = runDaemonMode(os.Args)
	case "ui":
		err = runUI()
	case "cli":
		err = runCLI(os.Args)
	default:
		err = fmt.Errorf("unknown mode: %s", *mode)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runDaemonMode parses daemon-specific flags and starts the loop.
func runDaemonMode(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)

	device := fs.String("device", "auto", "device path")
	interval := fs.Duration("interval", 5*time.Second, "poll interval")
	baud := fs.Int("baud", 115200, "baud rate")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return RunDaemon(ctx, DaemonConfig{
		Device:   *device,
		Baud:     *baud,
		Interval: *interval,
		Log:      os.Stdout,
	})
}
