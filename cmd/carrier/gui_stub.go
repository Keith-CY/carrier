//go:build !gui

package main

import (
	"fmt"
	"os"
)

func runGUI() {
	fmt.Fprintln(os.Stderr, "carrier: GUI not available in this build (compile with -tags gui to enable)")
	fmt.Fprintln(os.Stderr, "Use 'carrier daemon' to start the daemon server.")
	os.Exit(1)
}
