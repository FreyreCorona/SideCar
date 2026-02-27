package main

import (
	"flag"
	"os"
)

func main() {
	ui := flag.Bool("ui", false, "run in ui mode")

	if *ui {
		if err := runUI(); err != nil {
			os.Stderr.Write([]byte(err.Error()))
		}
		return
	}

	if err := runCLI(os.Args); err != nil {
		os.Stderr.Write([]byte(err.Error()))
	}
}
