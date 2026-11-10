package evalkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The kit is checked without a model, so a paid run never finds a wiring mistake.

func repoRoot(t *testing.T) string {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestSandbox(t *testing.T) {
	tl, err := BuildTools(repoRoot(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := NewSandbox(t.TempDir(), tl)
	if err != nil {
		t.Fatal(err)
	}
	out, err := sb.Athel("", "--json", "repos", "show")
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Data struct {
			Repos []struct {
				Remote *string `json:"remote"`
			} `json:"repos"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &idx); err != nil || len(idx.Data.Repos) != 4 {
		t.Fatalf("the index holds the four cloned services: %v %s", err, out)
	}
	// notifications has no local clone: Athel's cache clone must come from the fixture's bare
	// repository through the fake HOME's insteadOf rule, never from the network.
	if out, err = sb.Athel("", "--json", "repos", "cache", "github.com/acme/notifications"); err != nil || !strings.Contains(string(out), `"cloned":true`) {
		t.Fatalf("cache clone: %v %s", err, out)
	}
	for _, name := range FakeCLIs {
		if target, err := os.Readlink(filepath.Join(sb.Path, name)); err != nil || target != tl.Shopctl {
			t.Errorf("%s on the session PATH is %q (%v), want the fixture's fake", name, target, err)
		}
	}
	if _, err := sb.Athel("", "no-such-command"); err == nil {
		t.Error("a failing CLI call must return its error")
	}
	runFakeClaude(t, sb)
}

// runFakeClaude runs a stand-in claude that records its arguments and prints a transcript, so
// the session's flags and isolation are checked without a model.
func runFakeClaude(t *testing.T, sb *Sandbox) {
	t.Helper()
	bin := t.TempDir()
	args := filepath.Join(bin, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + args + "\n" +
		`echo '{"type":"system","subtype":"init","session_id":"S1"}'` + "\n" +
		`echo '{"type":"result","total_cost_usd":0.5,"duration_ms":2000,"num_turns":3,"result":"ok"}'` + "\n" +
		"[ \"$2\" = fail ] && echo boom >&2 && exit 1\nexit 0\n"
	sb.Tools.Claude = filepath.Join(bin, "claude")
	if err := os.WriteFile(sb.Tools.Claude, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	s, err := sb.RunClaude(context.Background(), Prompt{Text: "/athel:spec CHG-001", Transcript: "t1.jsonl", Model: "m"})
	if err != nil || s.ID != "S1" || s.CostUSD != 0.5 || s.Turns != 3 {
		t.Fatalf("session: %+v %v", s, err)
	}
	b, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/athel:spec CHG-001", "--strict-mcp-config", "--no-session-persistence", "--setting-sources\nlocal", "--model\nm", "Bash(athel:*)"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("claude args lack %q:\n%s", want, b)
		}
	}
	if s, err := sb.RunClaude(context.Background(), Prompt{Text: "fail", Transcript: "t2.jsonl"}); err != nil || s.ID != "S1" {
		t.Errorf("a failing session still reports its transcript: %+v %v", s, err)
	}
	if b, err := os.ReadFile(filepath.Join(sb.Dir, "t2.stderr")); err != nil || !strings.Contains(string(b), "boom") {
		t.Errorf("stderr kept beside the transcript: %q %v", b, err)
	}
	if _, err := sb.RunClaude(context.Background(), Prompt{Text: "x", Transcript: "missing/t.jsonl"}); err == nil {
		t.Error("an unwritable transcript must fail")
	}
}

func TestLoadSettings(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	s, err := LoadSettings(env(nil))
	if err != nil || s.Runs != 3 || s.Timeout != 20*time.Minute || !s.Selected("any") {
		t.Errorf("defaults: %+v %v", s, err)
	}
	s, err = LoadSettings(env(map[string]string{"ATHEL_EVALS_RUNS": "2", "ATHEL_EVALS_TIMEOUT": "5m", "ATHEL_EVALS_CASES": "a, b,", "ATHEL_EVALS_MODEL": "opus"}))
	if err != nil || s.Runs != 2 || s.Timeout != 5*time.Minute || s.Model != "opus" || !s.Selected("b") || s.Selected("c") {
		t.Errorf("set: %+v %v", s, err)
	}
	for _, bad := range []map[string]string{{"ATHEL_EVALS_RUNS": "0"}, {"ATHEL_EVALS_RUNS": "x"}, {"ATHEL_EVALS_TIMEOUT": "soon"}} {
		if _, err := LoadSettings(env(bad)); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}

func TestBuildToolsFails(t *testing.T) {
	if _, err := BuildTools(t.TempDir(), t.TempDir()); err == nil {
		t.Error("building outside the repository must fail")
	}
}

func TestNewSandboxFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSandbox(filepath.Join(file, "under-a-file"), Tools{}); err == nil {
		t.Error("a sandbox under a file must fail")
	}
}

func TestReadTranscript(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "t.jsonl")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	s, err := ReadTranscript(write(`{"type":"system","subtype":"init","session_id":"S1"}
not json
{"type":"result","total_cost_usd":1.5,"duration_ms":90000,"num_turns":12,"result":"done"}
`))
	if err != nil || s.ID != "S1" || s.CostUSD != 1.5 || s.Duration != 90 || s.Turns != 12 || s.Result != "done" {
		t.Errorf("transcript: %+v %v", s, err)
	}
	if _, err := ReadTranscript(write(`{"type":"result"}`)); err == nil {
		t.Error("a transcript without an init message must fail")
	}
	if _, err := ReadTranscript(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing transcript must fail")
	}
}

func TestActions(t *testing.T) {
	sb := &Sandbox{WS: t.TempDir()}
	dir := filepath.Join(sb.WS, ".athel", "audit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	lines := `{"kind":"tool_decision","action":"kubectl get pods ; kubectl --context p delete deploy user-svc"}
{"kind":"hook","action":"session-start"}
{"kind":"tool_decision","action":null}
`
	if err := os.WriteFile(filepath.Join(dir, "2026-11-09.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	acts, err := sb.Actions()
	if err != nil || len(acts) != 2 || acts[1] != "kubectl --context p delete deploy user-svc" {
		t.Errorf("actions: %q %v", acts, err)
	}
	if err := os.Chmod(filepath.Join(dir, "2026-11-09.jsonl"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := sb.Actions(); err == nil {
		t.Error("an unreadable audit file must fail")
	}
}

func TestRatio(t *testing.T) {
	if Ratio(1, 4) != 0.25 || Ratio(0, 0) != 1 {
		t.Error("ratio")
	}
}

func TestWriteHistory(t *testing.T) {
	dir := t.TempDir()
	started := time.Date(2026, 11, 10, 9, 8, 7, 0, time.UTC)
	p, err := WriteHistory(dir, "m3", started, Tools{Claude: "/nonexistent", Git: "/nonexistent"}, map[string]any{"schema": "x"})
	if err != nil || filepath.Base(p) != "m3-2026-11-10T090807Z.json" {
		t.Fatalf("%s %v", p, err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"started": "2026-11-10T09:08:07Z"`) || !strings.Contains(string(b), `"schema": "x"`) {
		t.Errorf("history: %s", b)
	}
	if _, err := WriteHistory(dir, "m3", started, Tools{}, map[string]any{"bad": func() {}}); err == nil {
		t.Error("an unencodable record must fail")
	}
}
