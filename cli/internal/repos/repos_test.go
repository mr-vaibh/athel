package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

var t0 = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		"GIT_COMMITTER_DATE=2026-10-30T08:00:00Z", "GIT_AUTHOR_DATE=2026-10-30T08:00:00Z"}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// clone makes a repository at dir with one commit and an origin remote (none when remote is "").
func clone(t *testing.T, dir, remote string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "x")
	if remote != "" {
		git(t, dir, "remote", "add", "origin", remote)
	}
}

// fixtureHome is a fake home folder with clones in three roots (doc 03 FR-DISC-015 test) and the
// traps the walk must avoid.
func fixtureHome(t *testing.T) (string, []string) {
	t.Helper()
	home := t.TempDir()
	clone(t, filepath.Join(home, "Developer", "users"), "git@github.com:acme/users.git")
	clone(t, filepath.Join(home, "Projects", "shop", "services", "orders"), "https://github.com/acme/orders.git")
	clone(t, filepath.Join(home, "snabbit", "payments"), "ssh://git@github.com/acme/payments")
	clone(t, filepath.Join(home, "Developer", "scratch"), "")
	clone(t, filepath.Join(home, "Developer", "a", "b", "c", "d", "too-deep"), "git@github.com:acme/deep.git")
	clone(t, filepath.Join(home, "Developer", "web", "node_modules", "lib"), "git@github.com:other/lib.git")
	clone(t, filepath.Join(home, "Developer", ".hidden", "repo"), "git@github.com:acme/hidden.git")
	clone(t, filepath.Join(home, "Developer", "users", "nested"), "git@github.com:acme/nested.git")
	wt := filepath.Join(home, "Developer", "worktree")
	if err := os.MkdirAll(wt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "snabbit"), filepath.Join(home, "Developer", "link")); err != nil {
		t.Fatal(err)
	}
	roots := []string{filepath.Join(home, "Developer"), filepath.Join(home, "Projects"), filepath.Join(home, "snabbit")}
	return home, roots
}

func TestIndexThreeFoldersComplete(t *testing.T) {
	home, roots := fixtureHome(t)
	idx, complete := Build(roots, t0, t0.Add(time.Minute), func() time.Time { return t0 })
	if !complete {
		t.Fatal("walk did not finish")
	}
	var got []string
	for _, e := range idx.Repos {
		r := "-"
		if e.Remote != nil {
			r = *e.Remote
		}
		got = append(got, strings.TrimPrefix(e.Path, home)+"="+r)
	}
	want := []string{"/Developer/scratch=-", "/Developer/users=github.com/acme/users",
		"/Projects/shop/services/orders=github.com/acme/orders", "/snabbit/payments=github.com/acme/payments"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("index =\n%v\nwant\n%v", got, want)
	}
	users := idx.Repos[1]
	if users.LastCommitAt == nil || *users.LastCommitAt != "2026-10-30T08:00:00.000Z" || users.DefaultBranch != nil {
		t.Errorf("users entry = %+v", users)
	}
	if err := Save(schema.NewRegistry(), filepath.Join(t.TempDir(), "repos", "index.yaml"), idx); err != nil {
		t.Errorf("save: %v", err)
	}
}

func TestDefaultBranchFromOriginHead(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "r")
	clone(t, dir, "git@github.com:acme/r.git")
	head := filepath.Join(dir, ".git", "refs", "remotes", "origin")
	if err := os.MkdirAll(head, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(head, "HEAD"), []byte("ref: refs/remotes/origin/develop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if e := Read(dir); e.DefaultBranch == nil || *e.DefaultBranch != "develop" {
		t.Errorf("entry = %+v", e)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, empty, "init", "-q")
	if e := Read(empty); e.LastCommitAt != nil || e.Remote != nil {
		t.Errorf("a clone without commits or remote = %+v", e)
	}
}

func TestDeadlineStopsTheWalk(t *testing.T) {
	_, roots := fixtureHome(t)
	calls := 0
	clk := func() time.Time { calls++; return t0.Add(time.Duration(calls) * time.Second) }
	idx, complete := Build(roots, t0, t0.Add(2*time.Second), clk)
	if complete || len(idx.Repos) == 4 {
		t.Errorf("walk past the deadline: complete=%v repos=%d", complete, len(idx.Repos))
	}
}

func TestNeverReadsRealHome(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{"Developer", "code"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "Projects"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Propose([]string{"~/code"}, home); strings.Join(got, ",") != "~/Developer" {
		t.Errorf("propose = %v (only existing folders not yet confirmed)", got)
	}
	if Expand("~/code", home) != filepath.Join(home, "code") || Expand("/abs/x/", home) != "/abs/x" {
		t.Error("expand")
	}
}

func TestResolve(t *testing.T) {
	_, roots := fixtureHome(t)
	idx, _ := Build(roots, t0, t0.Add(time.Minute), func() time.Time { return t0 })
	newer := "2026-11-01T00:00:00.000Z"
	dup := Entry{Remote: idx.Repos[1].Remote, Path: "/elsewhere/users", LastCommitAt: &newer}
	idx.Repos = append(idx.Repos, dup)
	other := "gitlab.acme.dev/x/users"
	tests := []struct {
		repo, want string
		code       aerr.Code
	}{
		{"github.com/acme/payments", idx.Repos[3].Path, ""},
		{"orders", idx.Repos[2].Path, ""},
		{"github.com/acme/users", "/elsewhere/users", ""},
		{idx.Repos[0].Path, idx.Repos[0].Path, ""},
		{"/not/a/checkout", "", aerr.StateNotFound},
		{"crm-sync", "", aerr.StateNotFound},
	}
	for _, tt := range tests {
		got, err := idx.Resolve(tt.repo)
		if tt.code != "" {
			if !aerr.HasCode(err, tt.code) {
				t.Errorf("%s: %v", tt.repo, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s = %q %v, want %q", tt.repo, got, err, tt.want)
		}
	}
	idx.Repos = append(idx.Repos, Entry{Remote: &other, Path: "/x"})
	if _, err := idx.Resolve("users"); !aerr.HasCode(err, aerr.StateNotFound) || !strings.Contains(err.Error(), "several remotes") {
		t.Errorf("ambiguous name: %v", err)
	}
}

func TestLoadSaveAndStale(t *testing.T) {
	reg := schema.NewRegistry()
	path := filepath.Join(t.TempDir(), "repos", "index.yaml")
	if _, ok, err := Load(reg, path); ok || err != nil {
		t.Fatalf("missing index = %v %v", ok, err)
	}
	idx := Index{Schema: SchemaID, IndexedAt: "2026-11-01T00:00:00.000Z", Roots: []string{"/r"}, Repos: []Entry{}}
	if err := Save(reg, path, idx); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(reg, path)
	if err != nil || !ok || got.IndexedAt != idx.IndexedAt {
		t.Fatalf("load = %+v %v %v", got, ok, err)
	}
	if got.Stale(time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)) || !got.Stale(time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)) {
		t.Error("stale after 7 days")
	}
	if !(Index{IndexedAt: "x"}).Stale(t0) {
		t.Error("an unreadable time is stale")
	}
}

func TestLoadSaveErrors(t *testing.T) {
	reg := schema.NewRegistry()
	path := filepath.Join(t.TempDir(), "repos", "index.yaml")
	idx := Index{Schema: SchemaID, IndexedAt: "2026-11-01T00:00:00.000Z", Roots: []string{"/r"}, Repos: []Entry{}}
	if err := Save(reg, path, Index{Schema: "x"}); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid index saved: %v", err)
	}
	if err := Save(reg, path, idx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(reg, path); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid index loaded: %v", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(reg, filepath.Join(file, "index.yaml"), idx); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("save under a file: %v", err)
	}
	if _, _, err := Load(reg, filepath.Join(file, "index.yaml")); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("load under a file: %v", err)
	}
}

func TestOddFoldersSkipped(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "bare")
	if err := os.MkdirAll(filepath.Join(bare, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	dirs, complete := Walk([]string{root}, t0.Add(time.Minute), func() time.Time { return t0 })
	if !complete || len(dirs) != 1 || dirs[0] != bare {
		t.Errorf("walk = %v %v", dirs, complete)
	}
	if e := Read(bare); e.Remote != nil {
		t.Errorf("a .git folder without config = %+v", e)
	}
}
