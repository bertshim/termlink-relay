// Copyright 2026 Bert Shim <bertshim@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Command termlink is a single-binary WebSocket terminal relay that runs as a
// server, host, or client depending on the chosen subcommand.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bertshim/termlink-relay/internal/ptyclient"
	"github.com/bertshim/termlink-relay/internal/ptyhost"
	"github.com/bertshim/termlink-relay/internal/relay"
)

const usage = `termlink — WebSocket terminal relay (server-allocated code/PIN)

Usage:
  termlink server [-addr :9000]
      Run the relay server. It allocates a fresh numeric code and PIN per host.
  termlink host   -server ws://HOST:9000
      Share this machine's terminal (host). The relay returns the session
      code and PIN, which this command displays for clients to use.
  termlink client -server ws://HOST:9000 -code CODE -pin PIN
      Connect to a shared terminal (client), using the host's code and PIN.

The legacy -mode style also works:
  termlink -mode server -addr :9000
  termlink -mode host   -server ws://HOST:9000
  termlink -mode client -server ws://HOST:9000 -code 123456 -pin 654321

Environment:
  TERMLINK_SERVER   default relay URL (host/client, default ws://localhost:9000)
`

func main() {
	// Subcommand style. Falls back to legacy -mode flags when the first
	// argument is a flag (starts with "-") or absent.
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		switch os.Args[1] {
		case "server":
			cmdServer(os.Args[2:])
		case "host":
			cmdHost(os.Args[2:])
		case "client":
			cmdClient(os.Args[2:])
		case "help", "-h", "--help":
			fmt.Print(usage)
		default:
			fmt.Printf("unknown command %q\n\n%s", os.Args[1], usage)
			os.Exit(1)
		}
		return
	}

	legacy()
}

// legacy preserves the original -mode based CLI.
func legacy() {
	mode := flag.String("mode", "", "server | host | client")
	addr := flag.String("addr", ":9000", "listen address (server mode)")
	server := flag.String("server", "", "relay URL, e.g. ws://HOST:9000 (host/client mode)")
	code := flag.String("code", "", "session code from the host (client mode)")
	pin := flag.String("pin", "", "session PIN from the host (client mode)")
	flag.Parse()

	switch *mode {
	case "server":
		relay.Run(*addr)
	case "host":
		requireFlag("server", *server)
		ptyhost.Run(*server)
	case "client":
		requireFlag("server", *server)
		requireFlag("code", *code)
		requireFlag("pin", *pin)
		ptyclient.Run(*server, *code, *pin)
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", ":9000", "listen address")
	_ = fs.Parse(args)

	relay.Run(*addr)
}

func cmdHost(args []string) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	server := fs.String("server", defaultServer(), "relay URL, e.g. ws://HOST:9000")
	_ = fs.Parse(args)

	ptyhost.Run(*server)
}

func cmdClient(args []string) {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	server := fs.String("server", defaultServer(), "relay URL, e.g. ws://HOST:9000")
	code := fs.String("code", "", "session code from the host")
	pin := fs.String("pin", "", "session PIN from the host")
	_ = fs.Parse(args)

	requireFlag("code", *code)
	requireFlag("pin", *pin)
	ptyclient.Run(*server, *code, *pin)
}

func defaultServer() string {
	if v := os.Getenv("TERMLINK_SERVER"); v != "" {
		return v
	}
	return "ws://localhost:9000"
}

func requireFlag(name, value string) {
	if value == "" {
		log.Fatalf("-%s is required for this mode", name)
	}
}
