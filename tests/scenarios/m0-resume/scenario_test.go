// Package m0resume proves M0 exit criterion 1 against the built athel binary: a change is
// created, paused when its session ends, and resumed by a brand-new session with no context
// lost (doc 08 M0 exit criteria; F-15; UC-10).
package m0resume

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type athel struct {
	t       *testing.T
	bin, ws string
	env     []string
}

func (a *athel) run(session string, args ...string) map[string]any {
	a.t.Helper()
	cmd := exec.Command(a.bin, append([]string{"--json", "--workspace", a.ws}, args...)...)
	cmd.Env = append(a.env, "ATHEL_SESSION_ID="+session)
	out, err := cmd.Output()
	if err != nil {
		a.t.Fatalf("athel %v: %v\n%s", args, err, out)
	}
	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil || env["ok"] != true {
		a.t.Fatalf("athel %v: %s", args, out)
	}
	if data, ok := env["data"].(map[string]any); ok {
		return data
	}
	return nil
}

func (a *athel) hook(event, payload string) string {
	a.t.Helper()
	cmd := exec.Command(a.bin, "hook", event)
	cmd.Dir, cmd.Env, cmd.Stdin = a.ws, a.env, strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		a.t.Fatalf("hook %s: %v", event, err)
	}
	return string(out)
}

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "athel")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/mr-vaibh/athel/cli/cmd/athel").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// stable drops the only fields a resume may change: the lock holder and updated_at.
func stable(c map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range c {
		if k != "lock" && k != "updated_at" {
			out[k] = v
		}
	}
	return out
}

func TestScenarioCreatePauseResume(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the athel binary")
	}
	ws := t.TempDir()
	a := &athel{t: t, bin: build(t), ws: ws, env: append(os.Environ(), "ATHEL_HOME="+filepath.Join(ws, ".home"))}
	payload := func(session, extra string) string {
		return `{"session_id":"` + session + `","cwd":"` + ws + `"` + extra + `}`
	}

	// Session 1: create, move through states, work inside a step, then end.
	a.run("s1", "init")
	a.hook("session-start", payload("s1", `,"source":"startup"`))
	a.run("s1", "change", "new", "--title", "Phone update", "--prd-text", "# PRD\nLet customers update their phone number.")
	a.run("s1", "state", "enter", "CHG-001", "start")
	a.run("s1", "checkpoint", "set", "CHG-001", "start.prd-captured", "--note", "PRD captured")
	a.run("s1", "state", "exit", "CHG-001", "start", "--result", "ok")
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		a.run("s1", "state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "reached "+to, "--evidence", "prd/v1.md")
	}
	a.run("s1", "state", "enter", "CHG-001", "plan")
	a.run("s1", "checkpoint", "set", "CHG-001", "plan.tasks-drafted", "--note", "3 tasks drafted; ordering of T-02/T-03 open")
	a.run("s1", "budget", "add", "CHG-001", "--cost-usd", "3.5", "--tool-calls", "42")
	before := stable(a.run("s1", "change", "show", "CHG-001"))
	if out := a.hook("session-end", payload("s1", `,"reason":"prompt_input_exit"`)); !strings.Contains(out, "released locks: CHG-001") {
		t.Fatalf("session end did not release the lock: %s", out)
	}

	// Session 2: brand new, only the files on disk.
	start := a.hook("session-start", payload("s2", `,"source":"startup"`))
	if !strings.Contains(start, `CHG-001 \"Phone update\": planned, step plan running, last checkpoint plan.tasks-drafted`) {
		t.Errorf("SessionStart summary = %s", start)
	}
	brief := a.run("s2", "resume")
	after := stable(a.run("s2", "change", "show", "CHG-001"))
	if !reflect.DeepEqual(before, after) {
		t.Errorf("change.yaml changed across the pause:\nbefore %v\nafter  %v", before, after)
	}
	want := "continue step plan (attempt 1) from checkpoint plan.tasks-drafted (3 tasks drafted; ordering of T-02/T-03 open)"
	if brief["read_only"] != false || brief["next_action"] != want {
		t.Errorf("brief = %v", brief)
	}
	change := brief["change"].(map[string]any)
	if change["state"] != "planned" || change["lock"].(map[string]any)["session"] != "s2" {
		t.Errorf("resumed change = %v", change)
	}
	if h := after["history"].([]any); len(h) != 4 || h[3].(map[string]any)["evidence"].([]any)[0].(map[string]any)["sha256"] == nil {
		t.Errorf("history = %v", h)
	}
	prd, err := os.ReadFile(brief["prd_path"].(string))
	if err != nil || !strings.Contains(string(prd), "phone number") {
		t.Errorf("PRD = %q %v", prd, err)
	}

	// A third session at the same time only gets read access.
	if third := a.run("s3", "resume", "CHG-001"); third["read_only"] != true {
		t.Errorf("concurrent session got write access: %v", third)
	}
}
