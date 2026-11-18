package cli

import (
	"strings"
	"testing"
)

func (h *harness) stopHook(t *testing.T, active bool) map[string]any {
	t.Helper()
	payload(t, h, map[string]any{"session_id": "sess-A", "stop_hook_active": active})
	return hookOut(t, h, "stop")
}

// TestStopHookKeepsTheBuildGoing (doc 06 build loop step 7; doc 02 §Runtime model): a session may
// not stop while its build step's task is unfinished, at most 3 times in a row; then the step is
// blocked and a card asks the developer.
func TestStopHookKeepsTheBuildGoing(t *testing.T) {
	h, _ := goGates(t)
	if out := h.stopHook(t, false); strings.Contains(asJSON(t, out), "block") {
		t.Fatalf("no build step, no block: %v", out)
	}
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	if out := h.stopHook(t, true); strings.Contains(asJSON(t, out), `"decision":"block"`) {
		t.Errorf("never while Claude Code already continues for a stop hook: %v", out)
	}
	for n := 1; n <= 3; n++ {
		out := asJSON(t, h.stopHook(t, false))
		if !strings.Contains(out, `"decision":"block"`) || !strings.Contains(out, "Task T-01 is neither done nor blocked (stop "+itoa(n)+" of 3)") {
			t.Fatalf("stop %d: %s", n, out)
		}
	}
	if out := asJSON(t, h.stopHook(t, false)); strings.Contains(out, `"decision":"block"`) || !strings.Contains(out, "the step is blocked") {
		t.Errorf("the fourth stop is let through and the step blocked: %s", out)
	}
	if v := h.mustOK("state", "show", "CHG-001"); v["state"] != "blocked" {
		t.Errorf("the change is blocked: %v", v["state"])
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); !strings.Contains(open, "Task T-01 stopped 3 times without finishing") {
		t.Errorf("a card asks the developer: %s", open)
	}
}

func TestStopHookAfterTheTaskIsDone(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/edge_test.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.checked(t, "src/edge_test.go")
	h.reviewRound(t, "verdict: approved\n")
	h.mustOK("gate", "run", "--stage", "full", "CHG-001", "T-01")
	h.mustOK("build", "done", "CHG-001", "T-01")
	if out := asJSON(t, h.stopHook(t, false)); strings.Contains(out, `"decision":"block"`) {
		t.Errorf("a done task never blocks a stop: %s", out)
	}
}

// TestReadyForPRAndPRReady (FR-GIT-007): the change is ready for PR only when every task is done
// and every repo is pushed with its draft PR; the PR is marked ready only then, and only for
// the pushed head.
func TestReadyForPRAndPRReady(t *testing.T) {
	h, tree := goGates(t)
	h.stdin = plannedBlast()
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/edge_test.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.checked(t, "src/edge_test.go")
	h.reviewRound(t, "verdict: approved\n")
	h.mustOK("gate", "run", "--stage", "full", "CHG-001", "T-01")
	if _, env := h.runJSON("state", "transition", "CHG-001", "--to", "ready_for_pr", "--by", "athel", "--reason", "r"); !strings.Contains(asJSON(t, env), "not every plan task is done") {
		t.Errorf("before the task is done: %v", env)
	}
	h.mustOK("build", "done", "CHG-001", "T-01")
	h.mustOK("state", "exit", "CHG-001", "build", "--result", "ok")
	if _, env := h.runJSON("state", "transition", "CHG-001", "--to", "ready_for_pr", "--by", "athel", "--reason", "r"); !strings.Contains(asJSON(t, env), "users is not pushed") {
		t.Errorf("before the push: %v", env)
	}
	log := recordingGH(t, h)
	h.mustOK("git", "push", "CHG-001", "users")
	h.mustOK("pr", "open", "CHG-001", "users")
	if _, env := h.runJSON("pr", "ready", "CHG-001", "users"); !strings.Contains(asJSON(t, env), "only when the change is ready_for_pr") {
		t.Errorf("pr ready while building: %v", env)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "ready_for_pr", "--by", "athel", "--reason", "every task done")
	h.mustOK("pr", "update", "CHG-001", "users") // /athel:ship refreshes the body before marking it ready
	if _, env := h.runJSON("git", "push", "CHG-001", "users"); !strings.Contains(asJSON(t, env), "Athel pushes only while building") {
		t.Errorf("no push after building: %v", env)
	}
	h.mustOK("pr", "ready", "CHG-001", "users")
	if got := calls(t, log); !strings.HasPrefix(got[len(got)-1], "pr ready 901 --repo acme/users") {
		t.Errorf("gh pr ready: %v", got)
	}
	if out, err := commit(t, h, tree, "src/late.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, env := h.runJSON("pr", "ready", "CHG-001", "users"); !strings.Contains(asJSON(t, env), "is not the worktree's head") {
		t.Errorf("a commit not pushed: %v", env)
	}
	if _, env := h.runJSON("pr", "ready", "CHG-001", "orders"); errCode(env) != "E-STATE-NOT-FOUND" {
		t.Errorf("no PR there: %v", env)
	}
}
