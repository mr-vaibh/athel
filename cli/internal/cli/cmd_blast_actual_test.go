package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// pushBranch commits files on a new branch of the fixture's bare "GitHub" remote.
func pushBranch(t *testing.T, git func(string, ...string), bare, branch string, files map[string]string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "w")
	git(t.TempDir(), "clone", "-q", bare, work)
	git(work, "switch", "-q", "-c", branch)
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(work, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(work, "add", "-A")
	git(work, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-qm", "work on "+branch)
	git(work, "push", "-q", "origin", branch)
}

func TestBlastDiffAndCheck(t *testing.T) {
	h, _, src, git := cacheFixture(t)
	pushBranch(t, git, filepath.Join(src, "users.git"), "feature/phone", map[string]string{
		"api/phone.go":    "package api\n\nfunc reg(mux *M) { mux.HandleFunc(\"PUT /v1/users/{id}/phone\", f) }\n",
		"events/topic.go": "package events\n\nconst Topic = \"users.updated.v2\"\n",
	})
	base := defaultBranch(t, filepath.Join(src, "users.git"))
	s := h.mustOK("blast", "diff", "users", "--base", base, "--head", "feature/phone")
	if !strings.Contains(asJSON(t, s["routes"]), "PUT /v1/users/{id}/phone") || !strings.Contains(asJSON(t, s["events"]), "users.updated.v2") {
		t.Fatalf("surface: %v", s)
	}
	m3Spec(t, h)
	h.stdin = plannedBlast()
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	code, env := h.runJSON("blast", "check", "CHG-001", "users", "--base", base, "--head", "feature/phone")
	if code != aerr.ExitGate || errCode(env) != string(aerr.GateBlastDrift) || !strings.Contains(asJSON(t, env), "event users.updated.v2") {
		t.Fatalf("an unplanned event: %d %v", code, env)
	}
	md, err := os.ReadFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "blast-actual-github.com-acme-users.md"))
	if err != nil || !strings.Contains(string(md), "drift: the plan must be updated") {
		t.Errorf("actual report: %s %v", md, err)
	}
	h.stdin = strings.Replace(plannedBlast(), `{item: "PUT /v1/users/{id}"`, `{item: "users.updated.v2 event", kind: event, change: additive, service: users, evidence: [{kind: spec, ref: REQ-1}]}
  - {item: "PUT /v1/users/{id}/phone"`, 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	if v := h.mustOK("blast", "check", "CHG-001", "users", "--base", base, "--head", "feature/phone"); len(v["drift"].([]any)) != 0 {
		t.Errorf("a planned diff: %v", v)
	}
}

// defaultBranch is the branch a bare repo's HEAD names.
func defaultBranch(t *testing.T, bare string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", bare, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestBlastActualErrors(t *testing.T) {
	h, _, src, _ := cacheFixture(t)
	base := defaultBranch(t, filepath.Join(src, "users.git"))
	for _, args := range [][]string{{"blast", "diff", "users"}, {"blast", "check", "CHG-001", "users"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d, want usage", args, code)
		}
	}
	if code, env := h.runJSON("blast", "diff", "users", "--head", "no-such-branch"); code != aerr.ExitTool {
		t.Errorf("unknown ref: %d %v", code, env)
	}
	if code, env := h.runJSON("blast", "check", "CHG-001", "users", "--base", base, "--head", base); code == 0 || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("no planned report: %v", env)
	}
	h.stdin = strings.Replace(blastDoc, "ask: ASK-001", "change: CHG-001", 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	if code, env := h.runJSON("blast", "check", "CHG-001", "users", "--base", base, "--head", base); code == 0 || !strings.Contains(asJSON(t, env), "adhoc, not planned") {
		t.Errorf("an adhoc report: %v", env)
	}
}
