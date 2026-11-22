package gitw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// clone is a second clone of the remote, standing in for a teammate.
func (w world) clone(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mate")
	must(t, filepath.Dir(dir), "clone", "-q", w.remote, dir)
	return dir
}

func writeCommit(t *testing.T, dir, file, body, msg string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	must(t, dir, "add", "-A")
	must(t, dir, "commit", "-q", "-m", msg)
}

// started is a world with Athel's worktree and one commit on its branch, in src/.
func started(t *testing.T) (world, string) {
	t.Helper()
	w := newWorld(t)
	if _, err := w.add(t); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.ws, "repos", "svc@CHG-001")
	writeCommit(t, dir, "src/a.go", "package src\n", "feat: a\n\nAthel-Change: CHG-001")
	return w, dir
}

func owned(p string) bool { return strings.HasPrefix(p, "src/") }

func (w world) fetch(t *testing.T) {
	t.Helper()
	must(t, w.cache, "fetch", "-q", "--prune", "origin")
}

func TestPushFirstTime(t *testing.T) {
	w, dir := started(t)
	if ahead, err := w.tree.RemoteAhead(w.cache, dir, "athel/CHG-001-x"); err != nil || ahead {
		t.Fatalf("no remote branch yet: %v %v", ahead, err)
	}
	got, err := w.tree.Integrate(dir, "main", false, owned)
	if err != nil || got.Rebased || got.Merged {
		t.Fatalf("base unchanged: %+v %v", got, err)
	}
	if err := w.tree.Push(dir, "athel/CHG-001-x"); err != nil {
		t.Fatal(err)
	}
	if must(t, w.remote, "rev-parse", "athel/CHG-001-x") != must(t, dir, "rev-parse", "HEAD") {
		t.Error("the remote branch is not at HEAD")
	}
}

func TestIntegrateRebasesBeforeTheFirstPush(t *testing.T) {
	w, dir := started(t)
	mate := w.clone(t)
	writeCommit(t, mate, "docs/readme.md", "x\n", "docs: readme")
	must(t, mate, "push", "-q", "origin", "main")
	w.fetch(t)
	got, err := w.tree.Integrate(dir, "main", false, owned)
	if err != nil || !got.Rebased || got.Merged {
		t.Fatalf("%+v %v", got, err)
	}
	if parents := strings.Fields(must(t, dir, "log", "--format=%P", "-1")); len(parents) != 1 || parents[0] != must(t, w.remote, "rev-parse", "main") {
		t.Errorf("rebased onto the new base, linear: %v", parents)
	}
}

func TestIntegrateMergesAfterAPush(t *testing.T) {
	w, dir := started(t)
	if err := w.tree.Push(dir, "athel/CHG-001-x"); err != nil {
		t.Fatal(err)
	}
	mate := w.clone(t)
	writeCommit(t, mate, "docs/readme.md", "x\n", "docs: readme")
	must(t, mate, "push", "-q", "origin", "main")
	w.fetch(t)
	got, err := w.tree.Integrate(dir, "main", true, owned)
	if err != nil || got.Rebased || !got.Merged {
		t.Fatalf("a pushed branch is never rebased (history stays as pushed): %+v %v", got, err)
	}
	if parents := strings.Fields(must(t, dir, "log", "--format=%P", "-1")); len(parents) != 2 {
		t.Errorf("a merge commit: %v", parents)
	}
	if err := w.tree.Push(dir, "athel/CHG-001-x"); err != nil {
		t.Errorf("a fast-forward push after the merge: %v", err)
	}
}

func TestRemoteAheadWhenSomeoneElsePushed(t *testing.T) {
	w, dir := started(t)
	if err := w.tree.Push(dir, "athel/CHG-001-x"); err != nil {
		t.Fatal(err)
	}
	mate := w.clone(t)
	must(t, mate, "switch", "-q", "athel/CHG-001-x")
	writeCommit(t, mate, "src/b.go", "package src\n", "teammate's change")
	must(t, mate, "push", "-q", "origin", "athel/CHG-001-x")
	ahead, err := w.tree.RemoteAhead(w.cache, dir, "athel/CHG-001-x")
	if err != nil || !ahead {
		t.Errorf("someone else's commit on Athel's branch: %v %v", ahead, err)
	}
}

func TestIntegrateConflicts(t *testing.T) {
	t.Run("owned files are left for the developer", func(t *testing.T) {
		w, dir := started(t)
		mate := w.clone(t)
		writeCommit(t, mate, "src/a.go", "package src // theirs\n", "fix: theirs")
		must(t, mate, "push", "-q", "origin", "main")
		w.fetch(t)
		got, err := w.tree.Integrate(dir, "main", false, owned)
		if !aerr.HasCode(err, aerr.ConflictMerge) || strings.Join(got.Conflicts, ",") != "src/a.go" || len(got.Unowned) != 0 {
			t.Fatalf("%+v %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(must(t, dir, "rev-parse", "--git-dir"), "MERGE_HEAD")); err != nil {
			t.Errorf("the merge stays in progress for the developer to resolve: %v", err)
		}
		if _, err := w.tree.Integrate(dir, "main", false, owned); !aerr.HasCode(err, aerr.ConflictMerge) {
			t.Errorf("a merge in progress must be finished first: %v", err)
		}
	})
	t.Run("unowned files abort", func(t *testing.T) {
		w, dir := started(t)
		writeCommit(t, dir, "README.md", "ours\n", "docs: ours\n\nAthel-Change: CHG-001")
		mate := w.clone(t)
		writeCommit(t, mate, "README.md", "theirs\n", "docs: theirs")
		must(t, mate, "push", "-q", "origin", "main")
		w.fetch(t)
		got, err := w.tree.Integrate(dir, "main", false, owned)
		if !aerr.HasCode(err, aerr.ConflictMerge) || strings.Join(got.Unowned, ",") != "README.md" {
			t.Fatalf("%+v %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(must(t, dir, "rev-parse", "--git-dir"), "MERGE_HEAD")); err == nil {
			t.Error("a conflict in an unowned file is aborted, never resolved by Athel")
		}
		if out := must(t, dir, "status", "--porcelain"); out != "" {
			t.Errorf("the worktree is clean again: %q", out)
		}
	})
}

func TestPushFailures(t *testing.T) {
	w, dir := started(t)
	w.tree.Git = failingAt("push", 1, nil)
	if err := w.tree.Push(dir, "athel/CHG-001-x"); !aerr.HasCode(err, aerr.ToolGit) {
		t.Errorf("%v", err)
	}
	for _, word := range []string{"ls-remote", "is-ancestor", "fetch --quiet"} {
		w, dir := started(t)
		if err := w.tree.Push(dir, "athel/CHG-001-x"); err != nil {
			t.Fatal(err)
		}
		w.tree.Git = failingAt(word, 128, nil)
		_, errA := w.tree.RemoteAhead(w.cache, dir, "athel/CHG-001-x")
		_, errI := w.tree.Integrate(dir, "main", true, owned)
		if errA == nil && errI == nil {
			t.Errorf("failing %q must fail one of them", word)
		}
	}
}

// conflicted is a never-pushed branch whose README.md conflicts with the base.
func conflicted(t *testing.T) (world, string) {
	t.Helper()
	w, dir := started(t)
	writeCommit(t, dir, "README.md", "ours\n", "docs: ours\n\nAthel-Change: CHG-001")
	mate := w.clone(t)
	writeCommit(t, mate, "README.md", "theirs\n", "docs: theirs")
	must(t, mate, "push", "-q", "origin", "main")
	w.fetch(t)
	return w, dir
}

func TestIntegrateFailures(t *testing.T) {
	for _, word := range []string{"--git-path", "rebase --abort", "--diff-filter=U", "merge --abort"} {
		w, dir := conflicted(t)
		w.tree.Git = failingAt(word, 128, nil)
		if _, err := w.tree.Integrate(dir, "main", false, owned); !aerr.HasCode(err, aerr.ToolGit) {
			t.Errorf("failing %q: %v", word, err)
		}
	}
}
