//go:build nogui

package main

import (
	"fmt"
	"os"
)

func runGUI() {
	fmt.Fprintln(os.Stderr, "carrier: GUI not available in this build (compiled with -tags nogui)")
	fmt.Fprintln(os.Stderr, "Use 'carrier daemon' to start the daemon server.")
	os.Exit(1)
}
