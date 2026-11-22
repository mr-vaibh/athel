package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

var (
	buildOnce sync.Once
	builtExe  string
	errBuild  error
)

// athelBinary builds this athel once per test run, for the git hooks to call.
func athelBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "athel-bin-")
		if err != nil {
			errBuild = err
			return
		}
		builtExe = filepath.Join(dir, "athel")
		out, err := exec.Command("go", "build", "-o", builtExe, "github.com/mr-vaibh/athel/cli/cmd/athel").CombinedOutput()
		if err != nil {
			errBuild = &buildError{string(out)}
		}
	})
	if errBuild != nil {
		t.Fatal(errBuild)
	}
	return builtExe
}

type buildError struct{ out string }

func (e *buildError) Error() string { return "go build athel: " + e.out }

// worktreeWithHooks is an approved change with its users worktree, whose hooks call the real
// binary.
func worktreeWithHooks(t *testing.T) (*harness, string) {
	t.Helper()
	h := approvedOnUsers(t)
	h.exe = athelBinary(t)
	h.mustOK("git", "worktree", "CHG-001", "users")
	return h, filepath.Join(h.root, "repos", "users@CHG-001")
}

// commit stages file with body in tree and commits with msg through real git and its hooks.
func commit(t *testing.T, h *harness, tree, file, body, msg string, extra ...string) (string, error) {
	t.Helper()
	p := filepath.Join(tree, file)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + h.env["HOME"], "ATHEL_HOME=" + h.home,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=Athel", "GIT_AUTHOR_EMAIL=athel@example.invalid",
		"GIT_COMMITTER_NAME=Athel", "GIT_COMMITTER_EMAIL=athel@example.invalid"}
	for _, args := range [][]string{{"add", "-A"}, append([]string{"commit", "-q", "-m", msg}, extra...)} {
		cmd := exec.Command("git", append([]string{"-C", tree}, args...)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			return string(out), err
		}
	}
	return "", nil
}

const goodMsg = "feat(users): add phone check\n\nAthel-Change: CHG-001\nAthel-Task: T-01"

func TestGitHooksInstalled(t *testing.T) {
	h, _ := worktreeWithHooks(t)
	hooks := filepath.Join(h.root, ".athel", "git-hooks")
	for _, name := range gitHooks {
		b, err := os.ReadFile(filepath.Join(hooks, name))
		if err != nil || !strings.Contains(string(b), shellQuote(h.exe)+" git hook "+name) {
			t.Errorf("hook %s: %v\n%s", name, err, b)
		}
	}
}

// TestGitHooksRefuseBadCommits: real git runs the hooks, which call the real binary.
func TestGitHooksRefuseBadCommits(t *testing.T) {
	h, tree := worktreeWithHooks(t)
	secret := "package src\n\nconst token = \"ghp_0123456789abcdefghijABCDEFGHIJ012345\"\n"
	tests := []struct {
		name, file, body, msg string
		want                  []string
	}{
		{"bad message", "src/a.go", "package src\n", "add phone check", []string{"not conventional", "missing trailer Athel-Change"}},
		{"task outside the plan", "src/a.go", "package src\n", strings.Replace(goodMsg, "T-01", "T-07", 1), []string{"T-07 is not a task"}},
		{"secret", "src/b.go", secret, goodMsg, []string{"src/b.go adds what looks like a secret"}},
	}
	for _, tt := range tests {
		out, err := commit(t, h, tree, tt.file, tt.body, tt.msg)
		if err == nil || strings.Contains(out, "ghp_") {
			t.Errorf("%s: must be refused without quoting a secret: %v\n%s", tt.name, err, out)
		}
		for _, w := range tt.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: output lacks %q:\n%s", tt.name, w, out)
			}
		}
	}
}

func TestGitLintBeforePush(t *testing.T) {
	h, tree := worktreeWithHooks(t)
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("a good commit: %v\n%s", err, out)
	}
	if v := h.mustOK("git", "lint", "CHG-001", "users"); v["ok"] != true || len(v["commits"].([]any)) != 1 {
		t.Errorf("lint after a good commit: %v", v)
	}
	// A commit that slipped past the hooks (here made by the test itself) is caught before a push.
	if out, err := commit(t, h, tree, "src/c.go", "package src\n", "wip", "--no-verify"); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if code, env := h.runJSON("git", "lint", "CHG-001", "users"); code != aerr.ExitGate || errCode(env) != string(aerr.GateCommitLint) || !strings.Contains(asJSON(t, env), `subject \"wip\" is not conventional`) {
		t.Errorf("lint after a bad commit: %d %v", code, env)
	}
}

func TestGitHookCommandErrors(t *testing.T) {
	h, tree := worktreeWithHooks(t)
	for _, args := range [][]string{{"git", "hook"}, {"git", "hook", "commit-msg"}, {"git", "hook", "pre-commit", "x"}, {"git", "hook", "post-commit"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if _, env := h.runJSON("git", "hook", "pre-commit"); errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("outside a worktree: %v", env)
	}
	h.cwd = tree
	if _, env := h.runJSON("git", "hook", "commit-msg", "no-such-file"); errCode(env) != string(aerr.StateIO) {
		t.Errorf("a missing message file: %v", env)
	}
	if v := h.mustOK("git", "hook", "pre-commit"); v["ok"] != true {
		t.Errorf("nothing staged: %v", v)
	}
	h.cwd = filepath.Join(h.root, "repos", "users@CHG-009")
	if _, env := h.runJSON("git", "hook", "pre-commit"); errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("an unknown change's folder: %v", env)
	}
	h.cwd = ""
	if code, env := h.runJSON("git", "lint", "CHG-001", "orders"); errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("lint without a worktree: %d %v", code, env)
	}
	h.exe = ""
	other := approvedOnUsers(t)
	other.exe = ""
	if _, env := other.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.StateIO) {
		t.Errorf("no binary for the hooks: %v", env)
	}
}

func TestGitLintAndHookFailures(t *testing.T) {
	h, tree := worktreeWithHooks(t)
	for _, args := range [][]string{{"git", "lint", "CHG-001"}, {"git", "lint", "CHG-009", "users"}} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	if err := os.RemoveAll(filepath.Join(tree, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("git", "lint", "CHG-001", "users"); errCode(env) != string(aerr.ToolGit) {
		t.Errorf("a broken worktree: %v", env)
	}
	h.cwd = tree
	if _, env := h.runJSON("git", "hook", "pre-commit"); errCode(env) != string(aerr.ToolGit) {
		t.Errorf("the pre-commit hook in a broken worktree: %v", env)
	}
	blocked := approvedOnUsers(t)
	if err := os.WriteFile(filepath.Join(blocked.root, ".athel", "git-hooks"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := blocked.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.StateIO) {
		t.Errorf("a file where the hooks go: %v", env)
	}
}
