// Carrier — unified binary for the Carrier agent platform.
//
// Usage:
//
//	carrier            Launch the desktop GUI (default)
//	carrier daemon     Start the daemon HTTP API server
//	carrier --daemon   Same as above
//	carrier --help     Show usage
package main

import (
	"fmt"
	"os"

	"carrier/daemon/server"
)

const usage = `Carrier — unified agent platform binary

Usage:
  carrier              Launch the desktop GUI (requires -tags gui build)
  carrier daemon       Start the daemon HTTP API server
  carrier --daemon     Start the daemon HTTP API server
  carrier --help       Show this help message

Build with GUI:
  go build -tags gui -o carrier ./cmd/carrier/
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "daemon", "--daemon":
			server.Run()
			return
		case "--help", "-h", "help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
			fmt.Fprint(os.Stderr, usage)
			os.Exit(1)
		}
	}

	// Default: launch GUI
	runGUI()
}
