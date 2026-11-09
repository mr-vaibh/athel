package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
)

// BenchmarkPreToolUseInProcess measures the hook decision path inside one process: payload
// parse, workspace and config load, change lookup, throttled heartbeat, audit line, output.
// The end-to-end number including process start is tests/bench/hooklatency.
func BenchmarkPreToolUseInProcess(b *testing.B) {
	root, home := b.TempDir(), b.TempDir()
	env := func(k string) string {
		if k == "ATHEL_HOME" {
			return home
		}
		return ""
	}
	clk := clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))
	call := func(stdin string, args ...string) {
		var out, errOut bytes.Buffer
		if code := Run(Env{Args: args, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut,
			Getenv: env, Cwd: root, Clock: clk, Version: "bench", UserHome: func() (string, error) { return home, nil }}); code != 0 {
			b.Fatalf("athel %v: exit %d %s", args, code, errOut.String())
		}
	}
	call("", "init")
	call("", "change", "new", "--title", "bench", "--prd-text", "p")
	call("", "--session", "bench", "lock", "acquire", "CHG-001")
	payload, _ := json.Marshal(map[string]any{"session_id": "bench", "cwd": root, "hook_event_name": "PreToolUse",
		"tool_name": "Bash", "tool_input": map[string]any{"command": "kubectl get pods -n staging"}, "tool_use_id": "toolu_1"})
	b.ResetTimer()
	for b.Loop() {
		call(string(payload), "hook", "pre-tool-use")
	}
}
