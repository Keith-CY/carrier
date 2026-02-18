// Carrier — unified binary for the Carrier agent platform (Web UI mode).
//
// Usage:
//
//	carrier            Start the daemon HTTP API server (default)
//	carrier daemon     Start the daemon HTTP API server
//	carrier --daemon   Same as above
//	carrier --help     Show usage
package main

import (
	"fmt"
	"os"

	"carrier/daemon/server"
)

const usage = `Carrier — unified agent platform binary (Web UI)

Usage:
  carrier              Start the daemon HTTP API server (default)
  carrier daemon       Start the daemon HTTP API server
  carrier --daemon     Start the daemon HTTP API server
  carrier --help       Show this help message

Web UI architecture:
  - Browser-based UI connects to daemon/gateway APIs over localhost.
  - Desktop GUI runtime has been removed to keep runtime decoupled.
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

	// Default: run daemon for browser Web UI clients.
	server.Run()
}
