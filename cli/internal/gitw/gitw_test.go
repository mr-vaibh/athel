package gitw

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/repos"
)

var testEnv = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
	"GIT_AUTHOR_NAME=Dev", "GIT_AUTHOR_EMAIL=dev@example.invalid", "GIT_COMMITTER_NAME=Dev",
	"GIT_COMMITTER_EMAIL=dev@example.invalid", "GIT_AUTHOR_DATE=2026-11-01T10:00:00Z",
	"GIT_COMMITTER_DATE=2026-11-01T10:00:00Z", "LC_ALL=C"}

func run(dir string, args ...string) (int, string, error) { return gitro.RunEnv(testEnv, dir, args...) }

func must(t *testing.T, dir string, args ...string) string {
	t.Helper()
	code, out, err := run(dir, args...)
	if err != nil || code != 0 {
		t.Fatalf("git %v in %s: %d %v %s", args, dir, code, err, out)
	}
	return strings.TrimSpace(out)
}

// world is a remote with one commit on main, and Athel's cache clone of it, made the way M2 makes
// cache clones.
type world struct {
	remote, cache, ws string
	tree              Tree
}

func newWorld(t *testing.T) world {
	t.Helper()
	root := t.TempDir()
	work, remote := filepath.Join(root, "work"), filepath.Join(root, "remote.git")
	must(t, root, "init", "-q", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	must(t, work, "add", "-A")
	must(t, work, "commit", "-q", "-m", "first")
	must(t, root, "clone", "-q", "--bare", work, remote)
	c := repos.Cache{Dir: filepath.Join(root, "ws", "cache"), Git: run}
	cache, _, err := c.Ensure("example.invalid/acme/svc", remote, "")
	if err != nil {
		t.Fatal(err)
	}
	return world{remote: remote, cache: cache, ws: filepath.Join(root, "ws"), tree: Tree{Git: run}}
}

// add makes the worktree of CHG-001 on the wanted branch athel/CHG-001-x.
func (w world) add(t *testing.T) (Added, error) {
	t.Helper()
	return w.tree.Add(Request{Cache: w.cache, Dir: filepath.Join(w.ws, "repos", "svc@CHG-001"), Base: "main",
		Branch: "athel/CHG-001-x", Change: "CHG-001", Prefix: "athel/"})
}

func TestBranchName(t *testing.T) {
	tests := []struct{ template, change, title, want string }{
		{"athel/{change_id}-{slug}", "CHG-042", "Phone update", "athel/CHG-042-phone-update"},
		{"athel/{change_id}-{slug}", "CHG-001", "  Make payments  more reliable!! ", "athel/CHG-001-make-payments-more-reliable"},
		{"athel/{change_id}-{slug}", "CHG-001", "Ünïcode & symbols: 100%", "athel/CHG-001-n-code-symbols-100"},
		{"athel/{change_id}-{slug}", "CHG-001", "???", "athel/CHG-001-change"},
		{"athel/{change_id}-{slug}", "CHG-001", strings.Repeat("word ", 20), "athel/CHG-001-word-word-word-word-word-word-word-word"},
		{"feature/{change_id}", "CHG-007", "anything", "feature/CHG-007"},
	}
	for _, tt := range tests {
		if got := BranchName(tt.template, tt.change, tt.title); got != tt.want {
			t.Errorf("BranchName(%q, %q) = %q, want %q", tt.template, tt.title, got, tt.want)
		}
	}
	if p := Prefix("athel/{change_id}-{slug}"); p != "athel/" {
		t.Errorf("prefix %q", p)
	}
	if p := Prefix("{change_id}"); p != "" {
		t.Errorf("a template with no fixed prefix has none: %q", p)
	}
}

func TestWorktreeFromCacheNotClone(t *testing.T) {
	w := newWorld(t)
	got, err := w.add(t)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.ws, "repos", "svc@CHG-001")
	if got.Branch != "athel/CHG-001-x" || got.Reused || got.BaseHead != must(t, w.remote, "rev-parse", "main") {
		t.Errorf("added %+v", got)
	}
	if b := must(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); b != "athel/CHG-001-x" {
		t.Errorf("worktree is on %q", b)
	}
	common, err := filepath.EvalSymlinks(must(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(w.cache); common != want {
		t.Errorf("the worktree belongs to %s, not the cache clone %s", common, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Errorf("files not checked out: %v", err)
	}
}

// TestFetchKeepsAthelBranches: the cache fetches with --prune and a refspec that mirrors every
// remote branch, which would delete Athel's unpushed branch. Add excludes Athel's prefix first.
func TestFetchKeepsAthelBranches(t *testing.T) {
	w := newWorld(t)
	if _, err := w.add(t); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.ws, "repos", "svc@CHG-001")
	must(t, dir, "commit", "-q", "--allow-empty", "-m", "work\n\nAthel-Change: CHG-001")
	work := must(t, dir, "rev-parse", "HEAD")
	c := repos.Cache{Dir: filepath.Dir(w.cache), Git: run}
	if _, _, err := c.Ensure("example.invalid/acme/svc", w.remote, ""); err != nil {
		t.Fatal(err)
	}
	if got := must(t, w.cache, "rev-parse", "refs/heads/athel/CHG-001-x"); got != work {
		t.Errorf("after a fetch the branch is at %s, want %s", got, work)
	}
	if _, err := w.tree.Protect(w.cache, "athel/"); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(must(t, w.cache, "config", "--get-all", "remote.origin.fetch"), "^refs/heads/athel/*"); n != 1 {
		t.Errorf("protection added %d times", n)
	}
}

func TestWorktreePathMustBeFree(t *testing.T) {
	w := newWorld(t)
	if _, err := w.add(t); err != nil {
		t.Fatal(err)
	}
	if _, err := w.add(t); !aerr.HasCode(err, aerr.ConflictWorktree) {
		t.Errorf("a second add on the same path: %v", err)
	}
}

// pushRemoteBranch puts branch on the remote with one commit whose message is msg.
func pushRemoteBranch(t *testing.T, w world, branch, msg string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "other")
	must(t, filepath.Dir(tmp), "clone", "-q", w.remote, tmp)
	must(t, tmp, "switch", "-q", "-c", branch)
	must(t, tmp, "commit", "-q", "--allow-empty", "-m", msg)
	must(t, tmp, "push", "-q", "origin", branch)
}

func TestRemoteBranchNameCollision(t *testing.T) {
	t.Run("someone else's branch gets a suffix", func(t *testing.T) {
		w := newWorld(t)
		pushRemoteBranch(t, w, "athel/CHG-001-x", "not ours")
		pushRemoteBranch(t, w, "athel/CHG-001-x-2", "not ours either")
		got, err := w.add(t)
		if err != nil || got.Branch != "athel/CHG-001-x-3" || got.Reused {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("an earlier run of this change is reused", func(t *testing.T) {
		w := newWorld(t)
		pushRemoteBranch(t, w, "athel/CHG-001-x", "feat: a\n\nAthel-Change: CHG-001\nAthel-Task: T-1")
		got, err := w.add(t)
		if err != nil || got.Branch != "athel/CHG-001-x" || !got.Reused {
			t.Fatalf("%+v %v", got, err)
		}
		dir := filepath.Join(w.ws, "repos", "svc@CHG-001")
		if msg := must(t, dir, "log", "-1", "--format=%s"); msg != "feat: a" {
			t.Errorf("reused worktree is at %q", msg)
		}
	})
	t.Run("another change's branch is not reused", func(t *testing.T) {
		w := newWorld(t)
		pushRemoteBranch(t, w, "athel/CHG-001-x", "feat: a\n\nAthel-Change: CHG-002")
		if got, err := w.add(t); err != nil || got.Branch != "athel/CHG-001-x-2" {
			t.Fatalf("%+v %v", got, err)
		}
	})
	t.Run("every suffix taken", func(t *testing.T) {
		w := newWorld(t)
		pushRemoteBranch(t, w, "athel/CHG-001-x", "not ours")
		for i := 2; i <= maxSuffix; i++ {
			pushRemoteBranch(t, w, "athel/CHG-001-x-"+string(rune('0'+i)), "not ours")
		}
		if _, err := w.add(t); !aerr.HasCode(err, aerr.ConflictWorktree) {
			t.Errorf("%v", err)
		}
	})
}

// failingAt returns a git runner that fails the call whose arguments contain word.
func failingAt(word string, code int, err error) func(string, ...string) (int, string, error) {
	return func(dir string, args ...string) (int, string, error) {
		if strings.Contains(strings.Join(args, " "), word) {
			return code, "boom", err
		}
		return run(dir, args...)
	}
}

func TestAddGitFailures(t *testing.T) {
	for _, word := range []string{"--get-all", "--add", "--verify --quiet refs/heads/main", "ls-remote", "worktree add", "fetch --quiet"} {
		for _, kind := range []struct {
			code int
			err  error
		}{{128, nil}, {0, errors.New("no git")}} {
			w := newWorld(t)
			if word == "fetch --quiet" {
				pushRemoteBranch(t, w, "athel/CHG-001-x", "feat: a\n\nAthel-Change: CHG-001")
			}
			w.tree.Git = failingAt(word, kind.code, kind.err)
			if _, err := w.add(t); !aerr.HasCode(err, aerr.ToolGit) {
				t.Errorf("failing %q (%d, %v): %v", word, kind.code, kind.err, err)
			}
		}
	}
	w := newWorld(t)
	pushRemoteBranch(t, w, "athel/CHG-001-x", "feat: a\n\nAthel-Change: CHG-001")
	w.tree.Git = failingAt("log", 1, nil)
	if _, err := w.add(t); !aerr.HasCode(err, aerr.ToolGit) {
		t.Errorf("failing log: %v", err)
	}
	w = newWorld(t)
	if err := os.MkdirAll(filepath.Join(w.ws, "repos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.ws, "repos", "svc@CHG-001"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.add(t); !aerr.HasCode(err, aerr.ConflictWorktree) {
		t.Errorf("a file in the way: %v", err)
	}
}

func TestAddEdgeCases(t *testing.T) {
	w := newWorld(t)
	if changed, err := w.tree.Protect(w.cache, ""); changed || err != nil {
		t.Errorf("an empty prefix protects nothing: %v %v", changed, err)
	}
	if _, err := w.add(t); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.ws, "repos", "svc@CHG-001")
	must(t, w.cache, "worktree", "remove", dir)
	got, err := w.add(t)
	if err != nil || got.Branch != "athel/CHG-001-x-2" {
		t.Errorf("a local branch with no commits of this change is not reused: %+v %v", got, err)
	}
	blocked := newWorld(t)
	if err := os.WriteFile(filepath.Join(blocked.ws, "repos"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := blocked.add(t); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("an unreadable worktree path: %v", err)
	}
}

func TestMissingBaseBranch(t *testing.T) {
	w := newWorld(t)
	_, err := w.tree.Add(Request{Cache: w.cache, Dir: filepath.Join(w.ws, "repos", "svc@CHG-001"), Base: "trunk",
		Branch: "athel/CHG-001-x", Change: "CHG-001", Prefix: "athel/"})
	if !aerr.HasCode(err, aerr.StateNotFound) || !strings.Contains(err.Error(), "no base branch trunk") {
		t.Errorf("%v", err)
	}
}
