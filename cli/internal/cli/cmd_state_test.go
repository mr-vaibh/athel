package cli

import (
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func setup(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.mustOK("init")
	h.mustOK("change", "new", "--title", "Phone update", "--prd-text", "# PRD")
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	return h
}

func TestTransitionCommand(t *testing.T) {
	h := setup(t)
	v := h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "start", "--evidence", "prd/v1.md", "--prompt-id", "p-1")
	if v["state"] != "investigating" || v["lock"].(map[string]any)["session"] != "sess-A" {
		t.Errorf("view = %v", v)
	}
	show := h.mustOK("change", "show", "CHG-001")
	hist := show["history"].([]any)[0].(map[string]any)
	if hist["origin"].(map[string]any)["prompt_id"] != "p-1" || len(hist["evidence"].([]any)) != 1 {
		t.Errorf("history = %v", hist)
	}
	tests := []struct {
		name string
		args []string
		exit int
		code string
	}{
		{"illegal", []string{"CHG-001", "--to", "done", "--by", "human", "--reason", "r"}, aerr.ExitState, string(aerr.StateIllegalTransition)},
		{"missing --to", []string{"CHG-001", "--by", "human", "--reason", "r"}, aerr.ExitUsage, "USAGE"},
		{"no id", []string{"--to", "specified", "--by", "human", "--reason", "r"}, aerr.ExitUsage, "USAGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, env := h.runJSON(append([]string{"state", "transition"}, tt.args...)...)
			if code != tt.exit || errCode(env) != tt.code {
				t.Errorf("exit %d env %v", code, env)
			}
		})
	}
	h.env["ATHEL_SESSION_ID"] = ""
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "specified", "--by", "athel", "--reason", "r"); code != aerr.ExitState || errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: exit %d env %v", code, env)
	}
	if v := h.mustOK("--session", "sess-A", "state", "transition", "CHG-001", "--to", "specified", "--by", "athel", "--reason", "r"); v["state"] != "specified" {
		t.Errorf("--session flag: %v", v)
	}
}

func TestHumanOnlyThroughCLI(t *testing.T) {
	h := setup(t)
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	code, env := h.runJSON("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "athel", "--reason", "r")
	if code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyHumanRequired) {
		t.Errorf("exit %d env %v", code, env)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "approved in chat")
}

func TestStepCommands(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	v := h.mustOK("state", "enter", "CHG-001", "investigate")
	if v["step"].(map[string]any)["status"] != "running" {
		t.Errorf("enter = %v", v)
	}
	cp := h.mustOK("checkpoint", "set", "CHG-001", "investigate.sources-planned", "--note", "grafana + kubectl next")
	if cp["name"] != "investigate.sources-planned" || cp["step"] != "investigate" {
		t.Errorf("checkpoint = %v", cp)
	}
	shown := h.mustOK("checkpoint", "show", "CHG-001")
	if shown["last"].(map[string]any)["note"] != "grafana + kubectl next" || len(shown["checkpoints"].([]any)) != 1 {
		t.Errorf("checkpoint show = %v", shown)
	}
	v = h.mustOK("state", "exit", "CHG-001", "investigate", "--result", "blocked", "--reason", "kubectl login expired")
	if v["state"] != "blocked" || v["blocked_from"] != "investigating" {
		t.Errorf("exit blocked = %v", v)
	}
	if code, _ := h.runJSON("state", "enter", "CHG-001", "investigate", "--task", "bad"); code != aerr.ExitState {
		t.Errorf("bad task: exit %d", code)
	}
	if code, _ := h.runJSON("state", "exit", "CHG-001", "investigate", "--result", "ok"); code != aerr.ExitState {
		t.Errorf("exit of a stopped step: exit %d", code)
	}
	if code, _ := h.runJSON("checkpoint", "set", "CHG-001", "Bad Name"); code != aerr.ExitState {
		t.Errorf("bad checkpoint: exit %d", code)
	}
	empty := newHarness(t)
	empty.mustOK("init")
	empty.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	if shown := empty.mustOK("checkpoint", "show", "CHG-001"); shown["last"] != nil || len(shown["checkpoints"].([]any)) != 0 {
		t.Errorf("no checkpoints = %v", shown)
	}
}

func TestLockCommands(t *testing.T) {
	h := setup(t)
	if st := h.mustOK("lock", "acquire", "CHG-001"); st["session"] != "sess-A" || st["held"] != true {
		t.Errorf("acquire = %v", st)
	}
	h.clock.Advance(time.Minute)
	if r := h.mustOK("lock", "refresh", "CHG-001"); r["wrote"] != false {
		t.Errorf("throttled refresh = %v", r)
	}
	if r := h.mustOK("lock", "refresh", "CHG-001", "--force"); r["wrote"] != true {
		t.Errorf("forced refresh = %v", r)
	}
	code, env := h.runJSON("lock", "status")
	if list := env["data"].([]any); code != 0 || len(list) != 1 {
		t.Errorf("status all = %v", env)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-B"
	code, env = h.runJSON("lock", "acquire", "CHG-001")
	if code != aerr.ExitState || errCode(env) != string(aerr.StateLockHeld) || !strings.Contains(env["error"].(map[string]any)["message"].(string), "sess-A") {
		t.Errorf("second session: exit %d env %v", code, env)
	}
	h.clock.Advance(31 * time.Minute)
	if st := h.mustOK("lock", "acquire", "CHG-001", "--takeover", "--reason", "A crashed"); st["session"] != "sess-B" {
		t.Errorf("take-over = %v", st)
	}
	if r := h.mustOK("lock", "release", "--all-mine"); len(r["released"].([]any)) != 1 {
		t.Errorf("release all = %v", r)
	}
	if r := h.mustOK("lock", "release", "CHG-001"); len(r["released"].([]any)) != 0 {
		t.Errorf("release free = %v", r)
	}
	code, env = h.runJSON("lock", "status", "CHG-001")
	if st := env["data"].([]any)[0].(map[string]any); code != 0 || st["held"] != false {
		t.Errorf("status one = %v", env)
	}
}

func TestLockCommandErrors(t *testing.T) {
	h := setup(t)
	for _, args := range [][]string{{"lock", "release"}, {"lock", "release", "CHG-001", "--all-mine"}, {"lock", "status", "a", "b"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if code, _ := h.runJSON("lock", "refresh", "CHG-001"); code != aerr.ExitState {
		t.Errorf("refresh without lock: exit %d", code)
	}
	if code, _ := h.runJSON("lock", "status", "CHG-404"); code != aerr.ExitState {
		t.Errorf("status of missing change: exit %d", code)
	}
	if code, _ := h.runJSON("lock", "release", "CHG-404"); code != aerr.ExitState {
		t.Errorf("release of missing change: exit %d", code)
	}
}

func TestBudgetCommands(t *testing.T) {
	h := setup(t)
	b := h.mustOK("budget", "add", "CHG-001", "--cost-usd", "33", "--tool-calls", "5")
	if b["status"] != "warning" || b["used_cost_usd"] != 33.0 || len(b["events"].([]any)) != 1 {
		t.Errorf("budget add = %v", b)
	}
	if shown := h.mustOK("budget", "show", "CHG-001"); shown["used_tool_calls"] != 5.0 {
		t.Errorf("budget show = %v", shown)
	}
	if code, _ := h.runJSON("budget", "add", "CHG-001", "--minutes", "-1"); code != aerr.ExitState {
		t.Errorf("negative: exit %d", code)
	}
	if code, _ := h.runJSON("budget", "add", "CHG-001", "--cost-usd", "x"); code != aerr.ExitUsage {
		t.Errorf("not a number: exit %d", code)
	}
	if code, _ := h.runJSON("budget", "show", "CHG-404"); code != aerr.ExitState {
		t.Errorf("missing: exit %d", code)
	}
}

func TestResumeBrief(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "start investigating")
	h.mustOK("state", "enter", "CHG-001", "investigate")
	h.mustOK("checkpoint", "set", "CHG-001", "investigate.owners-found", "--note", "user-service owns phone")
	h.mustOK("lock", "release", "CHG-001")

	h.env["ATHEL_SESSION_ID"] = "sess-NEW"
	brief := h.mustOK("resume")
	if brief["read_only"] != false || !strings.Contains(brief["message"].(string), "now holds the lock") {
		t.Errorf("brief = %v", brief)
	}
	want := "continue step investigate (attempt 1) from checkpoint investigate.owners-found (user-service owns phone)"
	if brief["next_action"] != want {
		t.Errorf("next_action = %q", brief["next_action"])
	}
	if !strings.HasSuffix(brief["prd_path"].(string), "CHG-001/prd/v1.md") || len(brief["recent_history"].([]any)) != 1 {
		t.Errorf("brief = %v", brief)
	}

	h.env["ATHEL_SESSION_ID"] = "sess-OTHER"
	other := h.mustOK("resume", "CHG-001")
	if other["read_only"] != true || !strings.Contains(other["next_action"].(string), "read-only") {
		t.Errorf("second session = %v", other)
	}
	h.clock.Advance(31 * time.Minute)
	taken := h.mustOK("resume", "CHG-001", "--take-over", "--reason", "previous session ended")
	if taken["read_only"] != false {
		t.Errorf("take-over resume = %v", taken)
	}
}

func TestNextActionCases(t *testing.T) {
	h := setup(t)
	if b := h.mustOK("resume"); !strings.Contains(b["next_action"].(string), "no step running; next allowed states: investigating, blocked, cancelled") {
		t.Errorf("idle = %v", b["next_action"])
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "blocked", "--by", "athel", "--reason", "waiting for PRD answers")
	if b := h.mustOK("resume"); !strings.Contains(b["next_action"].(string), "resolve the blocker (waiting for PRD answers), then return to draft") {
		t.Errorf("blocked = %v", b["next_action"])
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "cancelled", "--by", "human", "--reason", "dropped")
	b := h.mustOK("resume", "CHG-001")
	if b["read_only"] != true || b["next_action"] != "none: CHG-001 is cancelled" {
		t.Errorf("cancelled = %v", b)
	}
	if code, _ := h.runJSON("resume"); code != aerr.ExitState {
		t.Errorf("no active change: exit %d", code)
	}
	if code, _ := h.runJSON("resume", "a", "b"); code != aerr.ExitUsage {
		t.Errorf("two ids: exit %d", code)
	}
}

func TestResumeListsOtherChangesAndRunningStepWithoutCheckpoint(t *testing.T) {
	h := setup(t)
	h.mustOK("change", "new", "--title", "Second", "--prd-text", "p")
	h.mustOK("state", "enter", "CHG-001", "start")
	b := h.mustOK("resume", "CHG-001")
	if b["next_action"] != "continue step start (attempt 1) from the start of the step" {
		t.Errorf("next_action = %v", b["next_action"])
	}
	others := b["other_active_changes"].([]any)
	if len(others) != 1 || others[0].(map[string]any)["id"] != "CHG-002" {
		t.Errorf("others = %v", others)
	}
	if code, _ := h.runJSON("resume", "CHG-404"); code != aerr.ExitState {
		t.Errorf("missing change: exit %d", code)
	}
}
