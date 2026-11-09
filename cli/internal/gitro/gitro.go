// Package gitro runs read-only git commands for the CLI: recipe checks and the repo index. Git is
// the one tool the helper CLI may run (doc 02: no network except git), and these calls must
// never change a developer's clone (FR-DISC-016).
package gitro

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
)

// Run runs git in dir and returns its exit code and combined output. System and global git config
// are ignored, so a developer's aliases, hooks or fsmonitor never run, and optional locks are off,
// so not even a clone's index is refreshed. An error means git itself could not be started.
func Run(dir string, args ...string) (int, string, error) {
	return RunEnv([]string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}, dir, args...)
}

// NetworkEnv is the environment for git commands that reach a remote with the developer's own
// credentials (FR-DISC-017): their git config (credential helpers) and SSH agent are kept, and
// git may never prompt.
func NetworkEnv(getenv func(string) string) []string {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C"}
	for _, k := range []string{"PATH", "HOME", "SSH_AUTH_SOCK", "XDG_CONFIG_HOME"} {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// RunEnv runs git in dir with exactly env.
func RunEnv(env []string, dir string, args ...string) (int, string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "--no-pager"}, args...)...) //nolint:gosec // fixed git subcommands; values are arguments, never shell
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), out.String(), nil
	}
	return 0, out.String(), err
}
