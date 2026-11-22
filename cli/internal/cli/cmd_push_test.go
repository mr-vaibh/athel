package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// building is a change in building on repo users, with hooks, a clean preflight and one good
// commit in its worktree; it returns the worktree, the bare remotes and a git runner.
func building(t *testing.T) (*harness, string, string, func(string, ...string)) {
	t.Helper()
	h, src, git := approvedOnUsersRemote(t)
	h.exe = athelBinary(t)
	fakeGH(t, h, "[]")
	h.mustOK("git", "preflight", "CHG-001", "users")
	h.mustOK("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "preflight clear")
	tree := filepath.Join(h.root, "repos", "users@CHG-001")
	if out, err := commit(t, h, tree, "src/a.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	return h, tree, src, git
}

// teammate clones the remote users repo for someone else's work.
func teammate(t *testing.T, src string, git func(string, ...string)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mate")
	git(filepath.Dir(dir), "clone", "-q", filepath.Join(src, "users.git"), dir)
	return dir
}

func mateCommit(t *testing.T, git func(string, ...string), dir, file, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	git(dir, "add", "-A")
	git(dir, "-c", "user.name=Mate", "-c", "user.email=mate@example.invalid", "commit", "-q", "-m", "mate")
}

func TestGitPush(t *testing.T) {
	h, tree, src, git := building(t)
	v := h.mustOK("git", "push", "CHG-001", "users")
	if v["branch"] != "athel/CHG-001-phone-update" || v["head"] == "" {
		t.Fatalf("push = %v", v)
	}
	mate := teammate(t, src, git)
	git(mate, "fetch", "-q", "origin", "athel/CHG-001-phone-update")
	if repos := h.mustOK("state", "show", "CHG-001")["repos"].([]any); repos[0].(map[string]any)["pushed"] != true {
		t.Errorf("pushed not recorded: %v", repos)
	}
	// The base moves: after a push Athel merges it in, never rebases.
	mateCommit(t, git, mate, "docs/x.md", "x\n")
	git(mate, "push", "-q", "origin", "main")
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if again := h.mustOK("git", "push", "CHG-001", "users"); again["merged"] != true || again["rebased"] != false {
		t.Errorf("second push = %v", again)
	}
}

func TestGitPushStopsOnForeignCommits(t *testing.T) {
	h, _, src, git := building(t)
	h.mustOK("git", "push", "CHG-001", "users")
	mate := teammate(t, src, git)
	git(mate, "switch", "-q", "athel/CHG-001-phone-update")
	mateCommit(t, git, mate, "src/mine.go", "package src\n")
	git(mate, "push", "-q", "origin", "athel/CHG-001-phone-update")
	code, env := h.runJSON("git", "push", "CHG-001", "users")
	if code == 0 || errCode(env) != string(aerr.ConflictForeignPush) {
		t.Fatalf("someone else's push: %d %v", code, env)
	}
	open := h.mustOK("decide", "list", "CHG-001", "--open")["decisions"].([]any)
	if len(open) != 1 || !strings.Contains(open[0].(map[string]any)["question"].(string), "Someone else pushed") {
		t.Errorf("a card asks the developer: %v", open)
	}
	if _, env := h.runJSON("git", "push", "CHG-001", "users"); errCode(env) != string(aerr.ConflictForeignPush) {
		t.Errorf("still stopped: %v", env)
	}
	if again := h.mustOK("decide", "list", "CHG-001", "--open")["decisions"].([]any); len(again) != 1 {
		t.Errorf("one card, not one per attempt: %v", again)
	}
}

func TestGitPushUnownedConflictRaisesCard(t *testing.T) {
	h, tree, src, git := building(t)
	if out, err := commit(t, h, tree, "README.md", "ours\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	mate := teammate(t, src, git)
	mateCommit(t, git, mate, "README.md", "theirs\n")
	git(mate, "push", "-q", "origin", "main")
	if _, env := h.runJSON("git", "push", "CHG-001", "users"); errCode(env) != string(aerr.ConflictMerge) || !strings.Contains(asJSON(t, env), "README.md") {
		t.Fatalf("conflict: %v", env)
	}
	if open := h.mustOK("decide", "list", "CHG-001", "--open")["decisions"].([]any); len(open) != 1 {
		t.Errorf("a card for the unowned conflict: %v", open)
	}
}

func TestGitPushRefusals(t *testing.T) {
	h, tree, _, _ := building(t)
	if out, err := commit(t, h, tree, "src/c.go", "package src\n", "wip", "--no-verify"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, env := h.runJSON("git", "push", "CHG-001", "users"); errCode(env) != string(aerr.GateCommitLint) {
		t.Errorf("a bad commit is never pushed: %v", env)
	}
	for _, args := range [][]string{{"git", "push", "CHG-001"}, {"git", "push", "CHG-009", "users"}, {"git", "push", "CHG-001", "orders"}} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	early := approvedOnUsers(t)
	if _, env := early.runJSON("git", "push", "CHG-001", "users"); errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("before building: %v", env)
	}
}

// TestGitPushNeverToProtected: the CLI checks the branch against git.yaml itself (FR-GIT-004),
// whatever change.yaml names; here a project override makes Athel's own branches protected.
func TestGitPushNeverToProtected(t *testing.T) {
	h, _, _, _ := building(t)
	override := "schema: athel/git@1\nprotected: [main, master, \"release/*\", \"hotfix/*\", \"athel/*\"]\n"
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "git.yaml"), []byte(override), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("git", "push", "CHG-001", "users"); errCode(env) != string(aerr.PolicyForbidden) {
		t.Errorf("a protected branch is never pushed: %v", env)
	}
}
