package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// approvedOnUsers takes the cache fixture's change to plan_approved with a plan whose one task
// is in the indexed repo users.
func approvedOnUsers(t *testing.T) *harness {
	t.Helper()
	h, _, _, _ := cacheFixture(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	m3Spec(t, h)
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "users-plan.yaml", strings.NewReplacer("repo: r,", "repo: users,", "verbs: [edit, test]", "verbs: [edit, test, commit, push]").Replace(m3PlanDoc)))
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
	return h
}

func TestGitWorktree(t *testing.T) {
	h := approvedOnUsers(t)
	v := h.mustOK("git", "worktree", "CHG-001", "users")
	tree := filepath.Join(h.root, "repos", "users@CHG-001")
	if v["branch"] != "athel/CHG-001-phone-update" || v["created"] != true || v["path"] != tree || v["remote"] != "github.com/acme/users" {
		t.Fatalf("worktree = %v", v)
	}
	if _, err := os.Stat(filepath.Join(tree, ".git")); err != nil {
		t.Fatalf("no worktree at %s: %v", tree, err)
	}
	if again := h.mustOK("git", "worktree", "CHG-001", "users"); again["created"] != false || again["branch"] != v["branch"] {
		t.Errorf("a second call returns the same worktree: %v", again)
	}
	repos := h.mustOK("state", "show", "CHG-001")["repos"].([]any)
	if len(repos) != 1 || repos[0].(map[string]any)["worktree"] != "repos/users@CHG-001" {
		t.Errorf("change.yaml repos = %v", repos)
	}
	// Rule 9 now knows the branch Athel created: pushing it is Athel's own, any other is not.
	push := func(branch string) map[string]any {
		return h.decide("Bash", map[string]any{"command": "git push origin " + branch}, tree)
	}
	if d := push("athel/CHG-001-phone-update"); d["decision"] != "allow" || d["rule"] != "9" {
		t.Errorf("own branch push = %v", d)
	}
	if d := push("athel/CHG-009-other"); d["decision"] != "deny" || d["code"] != string(aerr.PolicyForeignBranch) {
		t.Errorf("foreign branch push = %v", d)
	}
}

func TestGitWorktreeRefusals(t *testing.T) {
	h, _, _, _ := cacheFixture(t)
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "plan_approved or building") {
		t.Errorf("before approval: %d %v", code, env)
	}
	h = approvedOnUsers(t)
	if code, env := h.runJSON("git", "worktree", "CHG-001", "orders"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "no task of the approved plan") {
		t.Errorf("a repo outside the plan: %d %v", code, env)
	}
	if code, _ := h.runJSON("git", "worktree", "CHG-001"); code != aerr.ExitUsage {
		t.Errorf("missing repo: %d", code)
	}
	if code, _ := h.runJSON("git", "worktree", "CHG-009", "users"); code != aerr.ExitState {
		t.Errorf("unknown change: %d", code)
	}
	delete(h.env, "ATHEL_SESSION_ID")
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %d %v", code, env)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	if err := os.MkdirAll(filepath.Join(h.root, "repos", "users@CHG-001"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.ConflictWorktree) {
		t.Errorf("a path in the way: %d %v", code, env)
	}
}

// treeSum hashes every file under dir (its .git folder too): path, mode and content.
func treeSum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s %v %d\n", strings.TrimPrefix(p, dir), info.Mode(), info.ModTime().UnixNano())
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			_, _ = h.Write(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestDeveloperCloneUnchanged (FR-GIT-001): making Athel's worktree, which seeds the cache from
// the developer's clone, leaves that clone byte for byte as it was, timestamps included.
func TestDeveloperCloneUnchanged(t *testing.T) {
	h := approvedOnUsers(t)
	clone := filepath.Join(h.userHome, "Developer", "users")
	before := treeSum(t, clone)
	h.mustOK("git", "worktree", "CHG-001", "users")
	if after := treeSum(t, clone); after != before {
		t.Error("the developer's clone changed")
	}
}
