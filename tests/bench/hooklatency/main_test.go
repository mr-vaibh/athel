package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	var s []time.Duration
	for i := 1; i <= 100; i++ {
		s = append(s, time.Duration(i)*time.Millisecond)
	}
	tests := []struct {
		p    int
		want time.Duration
	}{{50, 50 * time.Millisecond}, {95, 95 * time.Millisecond}, {99, 99 * time.Millisecond}, {100, 100 * time.Millisecond}, {0, time.Millisecond}}
	for _, tt := range tests {
		if got := Percentile(s, tt.p); got != tt.want {
			t.Errorf("p%d = %v, want %v", tt.p, got, tt.want)
		}
	}
	if Percentile(nil, 95) != 0 {
		t.Error("empty samples")
	}
	if got := Percentile([]time.Duration{7}, 99); got != 7 {
		t.Errorf("single sample p99 = %v", got)
	}
}

func TestMeasure(t *testing.T) {
	ok := func([]byte) ([]byte, error) { return []byte(`{"permissionDecision":"allow"}`), nil }
	rep, err := Measure(ok, [][]byte{[]byte("{}")}, 50, 2, time.Second)
	if err != nil || rep.Runs != 50 || !rep.Pass || rep.BudgetP95Ms != 1000 {
		t.Errorf("rep = %+v, %v", rep, err)
	}
	slow := func([]byte) ([]byte, error) {
		time.Sleep(2 * time.Millisecond)
		return []byte(`"permissionDecision"`), nil
	}
	if rep, err := Measure(slow, [][]byte{nil}, 3, 0, time.Millisecond); err != nil || rep.Pass {
		t.Errorf("over budget must fail: %+v %v", rep, err)
	}
	failing := func([]byte) ([]byte, error) { return nil, errors.New("crash") }
	if _, err := Measure(failing, [][]byte{nil}, 1, 0, time.Second); err == nil {
		t.Error("spawn failure not reported")
	}
	if _, err := Measure(failing, [][]byte{nil}, 1, 1, time.Second); err == nil || !strings.Contains(err.Error(), "warmup") {
		t.Errorf("warmup failure: %v", err)
	}
	noDecision := func([]byte) ([]byte, error) { return []byte("{}"), nil }
	if _, err := Measure(noDecision, [][]byte{nil}, 1, 0, time.Second); err == nil {
		t.Error("answer without a decision accepted")
	}
	if _, err := Measure(ok, nil, 1, 0, time.Second); err == nil {
		t.Error("no payloads accepted")
	}
}

func TestPayloadsAreRealistic(t *testing.T) {
	ps := payloads("/ws", "s")
	if len(ps) != 5 {
		t.Fatalf("%d payloads", len(ps))
	}
	for _, p := range ps {
		for _, want := range []string{`"session_id":"s"`, `"hook_event_name":"PreToolUse"`, `"tool_input":{`, `"cwd":"/ws"`} {
			if !bytes.Contains(p, []byte(want)) {
				t.Errorf("payload %s lacks %s", p, want)
			}
		}
	}
}

// TestRunEndToEnd builds the real binary and measures a short run.
func TestRunEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the athel binary")
	}
	bin := filepath.Join(t.TempDir(), "athel")
	build := exec.Command("go", "build", "-o", bin, "github.com/mr-vaibh/athel/cli/cmd/athel")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	var out bytes.Buffer
	if code := run([]string{"-binary", bin, "-n", "20", "-warmup", "1", "-max-p95", "10s"}, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "hook latency over 20 runs") {
		t.Errorf("output = %s", out.String())
	}
	out.Reset()
	if code := run([]string{"-binary", bin, "-n", "5", "-warmup", "0", "-max-p95", "1ns"}, &out); code != 1 {
		t.Errorf("impossible budget: exit %d", code)
	}
	if code := run([]string{"-binary", filepath.Join(t.TempDir(), "missing")}, &out); code != 1 {
		t.Errorf("missing binary: exit %d", code)
	}
	if code := run(nil, &out); code != 2 {
		t.Errorf("no binary flag: exit %d", code)
	}
}

func TestMainUsage(t *testing.T) {
	if os.Getenv("HOOKLATENCY_MAIN") == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMainUsage")
	cmd.Env = append(os.Environ(), "HOOKLATENCY_MAIN=1")
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Errorf("main without flags: %v", err)
	}
}
