package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
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
	logFile := flag.String("log", "", "path to log file (default: stdout/stderr)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of sidecar:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nUse \"sidecar -mode cli -help\" for CLI commands help.\n")
	}

	flag.Parse()

	logOut := setupLogging(*logFile)
	if logOut != os.Stdout && logOut != os.Stderr {
		defer logOut.(io.Closer).Close()
	}

	var err error
	switch *mode {
	case "daemon":
		err = runDaemonMode(os.Args, logOut)
	case "ui":
		err = runUI(logOut)
	case "cli":
		err = runCLI(os.Args, logOut)
	default:
		err = fmt.Errorf("unknown mode: %s", *mode)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func setupLogging(path string) io.Writer {
	if path == "" {
		return os.Stdout
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", path, err)
		return os.Stdout
	}

	log.SetOutput(f)
	return f
}

// runDaemonMode parses daemon-specific flags and starts the loop.
func runDaemonMode(args []string, defaultLog io.Writer) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)

	device := fs.String("device", "auto", "device path")
	interval := fs.Duration("interval", 5*time.Second, "poll interval")
	baud := fs.Int("baud", 115200, "baud rate")
	logFile := fs.String("log", "", "path to log file (overrides global -log)")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	out := defaultLog
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open daemon log file: %w", err)
		}
		defer f.Close()
		out = f
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return RunDaemon(ctx, DaemonConfig{
		Device:   *device,
		Baud:     *baud,
		Interval: *interval,
		Log:      out,
	})
}
