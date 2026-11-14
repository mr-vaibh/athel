// Command athel is the deterministic helper CLI of the Athel Claude Code plugin.
// It owns state changes, locks, IDs, hashes and schema validation; it never calls an LLM.
package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

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
		ClaudeVersion: versionOf("claude"), Executable: os.Executable,
	}
}

// versionOf returns a probe that runs `<binary> --version` (local process, no network),
// because Claude Code does not pass its version to hooks. Bounded so SessionStart never hangs.
func versionOf(binary string) func() (string, error) {
	return func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, binary, "--version").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
}
