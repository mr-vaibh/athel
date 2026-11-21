package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// twoTasks adds T-02 to the test plan, after T-01.
var twoTasks = []string{"gates: []}", "gates: []}\n  - {id: T-02, title: y, repo: users, kind: internal, paths: [src/**], verbs: [edit, test, commit, push], covers: [AC-1], depends_on: [T-01], size: S, gates: []}"}

// TestBuildOrder (FR-BUILD-001): a task starts only when its dependencies are done.
func TestBuildOrder(t *testing.T) {
	h, _ := goGates(t, twoTasks...)
	if v := h.mustOK("build", "next", "CHG-001"); asJSON(t, v["ready"]) != `["T-01"]` {
		t.Fatalf("only T-01 is ready: %v", v)
	}
	if _, env := h.runJSON("state", "enter", "CHG-001", "build", "--task", "T-02"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "T-02 waits for T-01") {
		t.Errorf("T-02 before T-01: %v", env)
	}
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	if v := h.mustOK("build", "next", "CHG-001"); asJSON(t, v["in_progress"]) != `["T-01"]` || asJSON(t, v["ready"]) != `[]` {
		t.Errorf("T-01 in progress, nothing else ready: %v", v)
	}
}

// TestBuildManifests (FR-BUILD-002/006): the developer gets the task, its spec slice, the
// conventions, its decisions, its files and open findings; the reviewer gets the same base,
// the diff and the earlier rounds, never the developer's files list or findings to fix.
func TestBuildManifests(t *testing.T) {
	h, _ := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	h.reviewRound(t, majorFinding)
	dev := h.mustOK("build", "context", "CHG-001", "T-01")
	if dev["role"] != "developer" || !strings.Contains(asJSON(t, dev["acceptance_criteria"]), `"id":"AC-1"`) || len(dev["requirements"].([]any)) == 0 ||
		!strings.Contains(asJSON(t, dev["files"]), "src/ac_test.go") || !strings.Contains(asJSON(t, dev["open_findings"]), "R1-01") ||
		!strings.HasSuffix(dev["conventions"].(string), "conventions.md") || dev["diff"] != nil {
		t.Errorf("developer manifest: %v", dev)
	}
	rev := h.mustOK("review", "context", "CHG-001", "T-01")
	if rev["role"] != "reviewer" || rev["files"] != nil || rev["open_findings"] != nil || rev["diff"] != "reviews/T-01/diff.patch" ||
		asJSON(t, rev["rounds"]) != `["reviews/T-01/round-1.yaml"]` {
		t.Errorf("reviewer manifest: %v", rev)
	}
	dir := filepath.Join(h.root, ".athel", "changes", "CHG-001")
	for _, f := range []string{"build/T-01/manifest.yaml", "reviews/T-01/context.yaml", "reviews/T-01/diff.patch"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// TestBuildDone (FR-BUILD-007/009): done only with a green full run of the current commit, a done
// review and no card blocking the task; the task's commits are recorded.
func TestBuildDone(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/calc.go", "package src\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := commit(t, h, tree, "src/calc_test.go", "package src\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	_, env := h.runJSON("build", "done", "CHG-001", "T-01")
	if errCode(env) != string(aerr.GateRed) || !strings.Contains(asJSON(t, env), "no full gate run") || !strings.Contains(asJSON(t, env), "the review is not done") {
		t.Fatalf("not done yet: %v", env)
	}
	h.checked(t, "src/calc_test.go")
	h.reviewRound(t, "verdict: approved\n")
	h.mustOK("gate", "run", "--stage", "full", "CHG-001", "T-01")
	done := h.mustOK("build", "done", "CHG-001", "T-01")
	if done["status"] != "done" || len(done["commits"].([]any)) != 2 {
		t.Errorf("done with its two commits: %v", done)
	}
	if v := h.mustOK("build", "next", "CHG-001"); asJSON(t, v["done"]) != `["T-01"]` {
		t.Errorf("next: %v", v)
	}
	if out, err := commit(t, h, tree, "src/more.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, env := h.runJSON("build", "done", "CHG-001", "T-01"); errCode(env) != string(aerr.GateRed) {
		t.Errorf("a done task is not done again, and the full run is of an older commit: %v", env)
	}
}

// TestTaskWithoutCodeIsDoneAtOnce (ADR 027): a task with no paths changes no code, so it has no
// gates, review or reports; it is done at once, and its shared actions go to the developer in
// the PR body.
func TestTaskWithoutCodeIsDoneAtOnce(t *testing.T) {
	h, _ := goGates(t, "gates: []}\n", "gates: []}\n  - {id: T-02, title: confirm the window, repo: users, kind: internal, paths: [], verbs: [comment], covers: [AC-1], depends_on: [], size: S, gates: [], env_actions: [comment on the ticket with the agreed date]}\n"+
		"  - {id: T-03, title: ask the consumers, repo: notifications, kind: internal, paths: [], verbs: [comment], covers: [AC-1], depends_on: [], size: S, gates: [], env_actions: [ask who consumes the event]}\n")
	for _, task := range []string{"T-02", "T-03"} {
		h.mustOK("state", "enter", "CHG-001", "build", "--task", task)
		done := h.mustOK("build", "done", "CHG-001", task)
		if done["status"] != "done" || len(done["commits"].([]any)) != 0 {
			t.Errorf("%s done with no commits: %v", task, done)
		}
	}
	// notifications has no worktree and no PR, so its task's action goes to the PRs there are.
	body := asJSON(t, h.mustOK("pr", "body", "CHG-001", "users"))
	if !strings.Contains(body, "Shared actions for the developer") || !strings.Contains(body, "T-02: comment on the ticket with the agreed date") ||
		!strings.Contains(body, "T-03 (notifications): ask who consumes the event") {
		t.Errorf("the PR body lists the shared actions: %s", body)
	}
}
