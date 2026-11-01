// Command hooklatency measures the M0 exit criterion "hook path (Claude Code → hook → athel →
// decision) under 50 ms at p95" end to end: it spawns the real athel binary the way Claude Code
// runs an exec-form command hook (no shell, payload on stdin, JSON on stdout) and reports
// p50/p95/p99 over at least 1,000 invocations with realistic PreToolUse payloads.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"
)

// Report is the measurement result.
type Report struct {
	Runs        int     `json:"runs"`
	P50Ms       float64 `json:"p50_ms"`
	P95Ms       float64 `json:"p95_ms"`
	P99Ms       float64 `json:"p99_ms"`
	MaxMs       float64 `json:"max_ms"`
	BudgetP95Ms float64 `json:"budget_p95_ms"`
	Pass        bool    `json:"pass"`
}

// Spawner runs one hook invocation and returns its stdout.
type Spawner func(payload []byte) ([]byte, error)

// payloads are shaped like real Claude Code PreToolUse input (see the adapter's testdata).
func payloads(workspace, session string) [][]byte {
	tools := []struct {
		name  string
		input map[string]any
	}{
		{"Bash", map[string]any{"command": "kubectl get pods -n staging --context gke_acme-staging_eu", "description": "List pods", "timeout": 120000}},
		{"Read", map[string]any{"file_path": filepath.Join(workspace, "src", "profile", "phone.ts")}},
		{"Edit", map[string]any{"file_path": filepath.Join(workspace, "src", "profile", "phone.ts"), "old_string": "return null", "new_string": "throw new PhoneInvalidError()"}},
		{"Grep", map[string]any{"pattern": "UserUpdated", "path": workspace, "output_mode": "files_with_matches"}},
		{"mcp__grafana__query_prometheus", map[string]any{"expr": "sum(rate(http_requests_total{service=\"users\"}[5m]))"}},
	}
	out := make([][]byte, len(tools))
	for i, t := range tools {
		b, _ := json.Marshal(map[string]any{
			"session_id": session, "prompt_id": "550e8400-e29b-41d4-a716-446655440000",
			"transcript_path": filepath.Join(workspace, "transcript.jsonl"), "cwd": workspace,
			"permission_mode": "default", "hook_event_name": "PreToolUse", "tool_name": t.name,
			"tool_input": t.input, "tool_use_id": fmt.Sprintf("toolu_%04d", i),
		})
		out[i] = b
	}
	return out
}

// Measure runs n invocations (after warmup ones that are not counted) and checks each answer.
func Measure(spawn Spawner, inputs [][]byte, n, warmup int, budget time.Duration) (Report, error) {
	if n < 1 || len(inputs) == 0 {
		return Report{}, errors.New("need at least one run and one payload")
	}
	for i := range warmup {
		if _, err := spawn(inputs[i%len(inputs)]); err != nil {
			return Report{}, fmt.Errorf("warmup: %w", err)
		}
	}
	samples := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		out, err := spawn(inputs[i%len(inputs)])
		samples[i] = time.Since(start)
		if err != nil {
			return Report{}, fmt.Errorf("run %d: %w", i, err)
		}
		if !bytes.Contains(out, []byte(`"permissionDecision"`)) {
			return Report{}, fmt.Errorf("run %d: no decision in %q", i, out)
		}
	}
	slices.Sort(samples)
	r := Report{Runs: n, P50Ms: ms(Percentile(samples, 50)), P95Ms: ms(Percentile(samples, 95)),
		P99Ms: ms(Percentile(samples, 99)), MaxMs: ms(samples[n-1]), BudgetP95Ms: ms(budget)}
	r.Pass = Percentile(samples, 95) < budget
	return r, nil
}

// Percentile is the nearest-rank percentile of sorted samples.
func Percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100 // ceil(p/100 * n)
	return sorted[min(max(rank, 1), len(sorted))-1]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// execSpawner runs the binary like an exec-form command hook.
func execSpawner(binary string, env []string) Spawner {
	return func(payload []byte) ([]byte, error) {
		cmd := exec.Command(binary, "hook", "pre-tool-use")
		cmd.Stdin, cmd.Env = bytes.NewReader(payload), env
		return cmd.Output()
	}
}

// prepare creates a workspace with one change locked by the measuring session, so every
// invocation takes the realistic path: config load, change lookup, heartbeat check, audit line.
func prepare(binary, dir, session string, env []string) error {
	steps := [][]string{
		{"init"},
		{"change", "new", "--title", "Latency benchmark", "--prd-text", "# Benchmark PRD"},
		{"--session", session, "lock", "acquire", "CHG-001"},
	}
	for _, args := range steps {
		cmd := exec.Command(binary, append([]string{"--json", "--workspace", dir}, args...)...) //nolint:gosec // the binary under test is chosen by the operator
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("athel %v: %w: %s", args, err, out)
		}
	}
	return nil
}

func run(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("hooklatency", flag.ContinueOnError)
	binary := fs.String("binary", "", "path to the athel binary (required)")
	n := fs.Int("n", 1000, "measured invocations")
	warmup := fs.Int("warmup", 20, "uncounted warmup invocations")
	budget := fs.Duration("max-p95", 50*time.Millisecond, "p95 budget; exit 1 when exceeded")
	if err := fs.Parse(args); err != nil || *binary == "" {
		_, _ = fmt.Fprintln(stdout, "usage: hooklatency -binary PATH [-n 1000] [-warmup 20] [-max-p95 50ms]")
		return 2
	}
	dir, err := os.MkdirTemp("", "athel-hooklatency-")
	if err != nil {
		_, _ = fmt.Fprintln(stdout, "error:", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	env := append(os.Environ(), "ATHEL_HOME="+filepath.Join(dir, "home"))
	const session = "bench-session"
	if err := prepare(*binary, dir, session, env); err != nil {
		_, _ = fmt.Fprintln(stdout, "error:", err)
		return 1
	}
	rep, err := Measure(execSpawner(*binary, env), payloads(dir, session), *n, *warmup, *budget)
	if err != nil {
		_, _ = fmt.Fprintln(stdout, "error:", err)
		return 1
	}
	b, _ := json.Marshal(rep)
	_, _ = fmt.Fprintf(stdout, "hook latency over %d runs: p50 %.2f ms, p95 %.2f ms, p99 %.2f ms, max %.2f ms (budget p95 < %.0f ms)\n%s\n",
		rep.Runs, rep.P50Ms, rep.P95Ms, rep.P99Ms, rep.MaxMs, rep.BudgetP95Ms, b)
	if !rep.Pass {
		return 1
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
