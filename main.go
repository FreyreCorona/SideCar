package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main dispatches to the correct mode based on the first positional argument
// or the -mode flag for daemon/ui.
//
// Usage:
//
//	sidecar                          → CLI help
//	sidecar -cmd on                  → CLI
//	sidecar -mode daemon             → daemon
//	sidecar -mode daemon -interval 3s
//	sidecar -mode ui                 → UI webview
func main() {
	mode := detectMode(os.Args)

	var err error
	switch mode {
	case "daemon":
		err = runDaemonMode(os.Args)
	case "ui":
		err = runUI()
	default:
		err = runCLI(os.Args)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// detectMode inspects the args without consuming them to decide the mode.
func detectMode(args []string) string {
	for i, arg := range args[1:] {
		switch arg {
		case "daemon", "ui":
			return arg
		case "-mode", "--mode":
			if i+2 < len(args) {
				return args[i+2]
			}
		}
		if len(arg) >= 7 && arg[:7] == "--mode=" {
			return arg[7:] // --mode=daemon
		}
		if len(arg) >= 6 && arg[:6] == "-mode=" {
			return arg[6:] // -mode=daemon
		}
	}
	return "ui"
}

// runDaemonMode parses daemon-specific flags and starts the loop.
func runDaemonMode(args []string) error {
	// Simple manual parsing to avoid depending on the global flag package
	device := "auto"
	baud := 115200
	interval := 5 * time.Second

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-device" || arg == "--device":
			if i+1 < len(args) {
				device = args[i+1]
				i++
			}
		case len(arg) > 8 && arg[:8] == "-device=":
			device = arg[8:]
		case arg == "-interval" || arg == "--interval":
			if i+1 < len(args) {
				if d, err := time.ParseDuration(args[i+1]); err == nil {
					interval = d
					i++
				}
			}
		case len(arg) > 10 && arg[:10] == "-interval=":
			if d, err := time.ParseDuration(arg[10:]); err == nil {
				interval = d
			}
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return RunDaemon(ctx, DaemonConfig{
		Device:   device,
		Baud:     baud,
		Interval: interval,
		Log:      os.Stdout,
	})
}
