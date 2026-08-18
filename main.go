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
	args := os.Args[1:]

	// Manual arg parsing: extract -mode and -log globally, pass the rest
	// to the sub-command.  This avoids flag.FlagSet conflicts between the
	// global flags and sub-command flags.
	mode := "ui"
	logFile := ""
	var subArgs []string
	hasCmd := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case "-cmd":
			hasCmd = true
			subArgs = append(subArgs, args[i])
			if i+1 < len(args) {
				subArgs = append(subArgs, args[i+1])
				i++
			}
		case "-log":
			if i+1 < len(args) {
				logFile = args[i+1]
				i++
			}
		case "-help", "-h":
			// Only show global help if no -cmd is present
			if !hasCmd {
				printGlobalUsage()
				return
			}
			subArgs = append(subArgs, args[i])
		default:
			subArgs = append(subArgs, args[i])
		}
	}

	// Auto-detect CLI mode when -cmd is provided
	if hasCmd && mode == "ui" {
		mode = "cli"
	}

	logOut := setupLogging(logFile)
	if logOut != os.Stdout && logOut != os.Stderr {
		defer logOut.(io.Closer).Close()
	}

	var err error
	switch mode {
	case "daemon":
		err = runDaemonMode(subArgs, logOut)
	case "ui":
		err = runUI(logOut)
	case "cli":
		err = runCLI(subArgs, logOut)
	default:
		err = fmt.Errorf("unknown mode: %s", mode)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printGlobalUsage() {
	fmt.Fprintf(os.Stderr, `Usage of sidecar:
  sidecar                    UI (default)
  sidecar -mode ui           UI webview
  sidecar -mode daemon       daemon loop
  sidecar -cmd <command>     CLI commands (auto-detects CLI mode)
  sidecar -mode cli          CLI commands (explicit)

Global flags:
  -mode string    runtime mode: ui | daemon | cli (default "ui")
  -cmd string     command to run (auto-switches to CLI mode)
  -log string     path to log file (default: stdout/stderr)

Use "sidecar -cmd help" for CLI commands help.
Use "sidecar -mode daemon -help" for daemon flags.
`)
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
	fs := newFlagSet("daemon")

	device := fs.String("device", "auto", "device path")
	interval := fs.Duration("interval", 5*time.Second, "poll interval")
	baud := fs.Int("baud", 115200, "baud rate")
	logFile := fs.String("log", "", "path to log file (overrides global -log)")

	if err := fs.Parse(args); err != nil {
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

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
