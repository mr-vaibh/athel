package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// cleanupTask is a deferred removal after T-01, in the plan's own repo.
var cleanupTask = []string{"gates: []}", "gates: []}\n  - {id: T-02, title: Remove the old phone field, repo: users, kind: contract, deferred: true, " +
	"paths: [src/old/**], verbs: [edit, delete], covers: [AC-1], depends_on: [T-01], size: S, gates: [], rollback: revert}"}

// prView answers gh pr view 901 with state, and every other call as recordingGH does.
func prView(t *testing.T, h *harness, state string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1 $2 $3\" in\n" +
		"  'pr view 901') echo '{\"state\":\"" + state + "\",\"isDraft\":false,\"reviewDecision\":\"APPROVED\",\"mergedAt\":\"2026-11-24T10:00:00Z\",\"statusCheckRollup\":[]}' ;;\n" +
		"  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

// TestFollowupDraftLinksParent (M5-plan G3, A6): the cleanup is drafted only after every PR of
// the set is merged and the developer says the consumers are deployed, once.
func TestFollowupDraftLinksParent(t *testing.T) {
	h, _, _ := approvedOnUsersRemote(t, cleanupTask...)
	h.exe = athelBinary(t)
	fakeGH(t, h, "[]")
	h.mustOK("git", "preflight", "CHG-001", "users")
	h.mustOK("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "preflight clear")
	tree := filepath.Join(h.root, "repos", "users@CHG-001")
	if out, err := commit(t, h, tree, "src/a.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	recordingGH(t, h)
	h.mustOK("pr", "open", "CHG-001", "users")
	if set := asJSON(t, h.mustOK("set", "show", "CHG-001")); !strings.Contains(set, `"role":"provider_cleanup"`) {
		t.Fatalf("the approved set has the cleanup: %s", set)
	}
	prView(t, h, "OPEN")
	if code, env := h.runJSON("followup", "draft", "CHG-001"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "not merged yet: users") {
		t.Errorf("before the merge: %v", env)
	}
	prView(t, h, "MERGED")
	if code, env := h.runJSON("followup", "draft", "CHG-001"); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Errorf("without the developer's prompt: %v", env)
	}
	h.humanPrompt("/athel:followup CHG-001")
	v := h.mustOK("followup", "draft", "CHG-001")
	b, err := os.ReadFile(v["path"].(string))
	if err != nil || !strings.Contains(string(b), "- Remove the old phone field, in users (paths: src/old/**). Planned in CHG-001 as T-02.") ||
		!strings.Contains(string(b), "Follow-up of CHG-001") || !strings.HasPrefix(v["next"].(string), "/athel:start ") {
		t.Errorf("draft %v: %v\n%s", v, err, b)
	}
	if code, env := h.runJSON("followup", "draft", "CHG-001"); code != aerr.ExitPolicy {
		t.Errorf("the grant is used once: %v", env)
	}
	failingGH(t, h)
	if code, env := h.runJSON("followup", "draft", "CHG-001"); code != aerr.ExitTool || !strings.Contains(asJSON(t, env), "cannot confirm") {
		t.Errorf("host down: %d %v", code, env)
	}
}

func TestFollowupWithoutCleanup(t *testing.T) {
	h := approvedOnUsers(t)
	if code, env := h.runJSON("followup", "draft", "CHG-001"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "no deferred cleanup") {
		t.Errorf("no cleanup in the set: %v", env)
	}
}
