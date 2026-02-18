// Carrier — unified binary for the Carrier agent platform.
//
// Usage:
//
//	carrier           Start the daemon HTTP API server (with WebUI)
//	carrier daemon    Same as above
//	carrier --help    Show usage
package main

import (
	"fmt"
	"os"

	"carrier/daemon/server"
)

const usage = `Carrier — unified agent platform binary

Usage:
  carrier              Start the daemon HTTP API server (with WebUI)
  carrier daemon       Same as above
  carrier --help       Show this help message

The WebUI is served at http://localhost:<port>/ alongside the API.
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

	// Default: start daemon (which includes WebUI)
	server.Run()
}
