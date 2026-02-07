package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Si no hay subcomando o hay flags → CLI
	if len(args) < 2 || args[1] != "ui" {
		return runCLI(args)
	}

	// sidecar ui
	return runUI()
}
