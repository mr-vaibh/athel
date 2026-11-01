// Command athel is the deterministic helper CLI of the Athel Claude Code plugin.
// It owns state changes, locks, IDs, hashes and schema validation; it never calls an LLM.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

// exit is a variable so the entry point can be tested in-process.
var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is separated from main so the whole entry point is testable without exiting the process.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		if _, err := fmt.Fprintf(stdout, "athel %s\n", version); err != nil {
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintln(stderr, "usage: athel version"); err != nil {
		return 1
	}
	return 2
}
