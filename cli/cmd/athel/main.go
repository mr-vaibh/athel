// Command athel is the deterministic helper CLI of the Athel Claude Code plugin.
// It owns state changes, locks, IDs, hashes and schema validation; it never calls an LLM.
package main

import (
	"os"

	"github.com/mr-vaibh/athel/cli/internal/cli"
	"github.com/mr-vaibh/athel/cli/internal/clock"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

// exit is a variable so the entry point can be tested in-process.
var exit = os.Exit

func main() {
	exit(cli.Run(environment(os.Getwd)))
}

// environment wires the real process environment into the CLI. If the working directory is
// unreadable, "." still lets commands that name --workspace run.
func environment(getwd func() (string, error)) cli.Env {
	cwd, err := getwd()
	if err != nil {
		cwd = "."
	}
	return cli.Env{
		Args: os.Args[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Getenv: os.Getenv, Cwd: cwd, UserHome: os.UserHomeDir, Clock: clock.System{}, Version: version,
	}
}
