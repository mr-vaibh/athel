package repos

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
)

// testGit is the cache's git with a temporary HOME and no system config, so tests never read
// this machine's git settings.
func testGit(t *testing.T) func(string, ...string) (int, string, error) {
	t.Helper()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e"}
	return func(dir string, args ...string) (int, string, error) { return gitro.RunEnv(env, dir, args...) }
}

// upstream makes a bare "remote" repository and a developer clone of it.
func upstream(t *testing.T) (bare, dev string) {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	clone(t, work, "")
	bare, dev = filepath.Join(root, "users.git"), filepath.Join(root, "dev", "users")
	git(t, root, "clone", "-q", "--bare", work, bare)
	git(t, root, "clone", "-q", bare, dev)
	return bare, dev
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	code, out, err := gitro.RunEnv([]string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "HOME=" + t.TempDir()}, dir, args...)
	if err != nil || code != 0 {
		t.Fatalf("git %v: %d %s %v", args, code, out, err)
	}
	return strings.TrimSpace(out)
}

func TestCacheCloneFromLocalReference(t *testing.T) {
	bare, dev := upstream(t)
	c := Cache{Dir: filepath.Join(t.TempDir(), "cache"), Git: testGit(t)}
	p, cloned, err := c.Ensure("github.com/acme/users", bare, dev)
	if err != nil || !cloned || p != filepath.Join(c.Dir, "github.com__acme__users.git") {
		t.Fatalf("ensure = %s %v %v", p, cloned, err)
	}
	if _, err := os.Stat(filepath.Join(p, "objects", "info", "alternates")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the cache still depends on the developer's clone (alternates): %v", err)
	}
	if got := gitOut(t, p, "config", "remote.origin.fetch"); got != headsRefspec {
		t.Errorf("fetch refspec = %q", got)
	}
	if gitOut(t, p, "rev-parse", "main") != gitOut(t, dev, "rev-parse", "HEAD") {
		t.Error("cache main differs from the remote")
	}
}

func TestFetchBeforeUse(t *testing.T) {
	bare, _ := upstream(t)
	c := Cache{Dir: filepath.Join(t.TempDir(), "cache"), Git: testGit(t)}
	if _, _, err := c.Ensure("github.com/acme/users", bare, ""); err != nil {
		t.Fatal(err)
	}
	pusher := filepath.Join(t.TempDir(), "pusher")
	git(t, filepath.Dir(pusher), "clone", "-q", bare, pusher)
	if err := os.WriteFile(filepath.Join(pusher, "new.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(t, pusher, "add", "-A")
	git(t, pusher, "commit", "-q", "-m", "teammate change")
	git(t, pusher, "push", "-q", "origin", "HEAD:main")
	p, cloned, err := c.Ensure("github.com/acme/users", bare, "")
	if err != nil || cloned {
		t.Fatalf("second ensure = %v %v", cloned, err)
	}
	if gitOut(t, p, "rev-parse", "main") != gitOut(t, pusher, "rev-parse", "HEAD") {
		t.Error("the cache was not refreshed before use")
	}
}

func TestUnreachableReported(t *testing.T) {
	c := Cache{Dir: filepath.Join(t.TempDir(), "cache"), Git: testGit(t)}
	_, _, err := c.Ensure("github.com/acme/private", filepath.Join(t.TempDir(), "no-such-repo.git"), "")
	if !aerr.HasCode(err, aerr.ToolUnreachable) {
		t.Fatalf("err = %v, want E-TOOL-UNREACHABLE", err)
	}
	if _, err := os.Stat(c.Path("github.com/acme/private")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a failed clone left a cache folder behind")
	}
	failConfig := Cache{Dir: c.Dir, Git: func(dir string, args ...string) (int, string, error) {
		if len(args) > 2 && args[2] == "config" {
			return 1, "", nil
		}
		return testGit(t)(dir, args...)
	}}
	bare, _ := upstream(t)
	if _, _, err := failConfig.Ensure("github.com/acme/users", bare, ""); !aerr.HasCode(err, aerr.ToolUnreachable) {
		t.Errorf("configure failure: %v", err)
	}
	if _, err := os.Stat(c.Path("github.com/acme/users")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a cache clone that could not be configured was kept")
	}
	broken := Cache{Dir: c.Dir, Git: func(string, ...string) (int, string, error) { return 0, "", errors.New("git not installed") }}
	if _, _, err := broken.Ensure("github.com/acme/x", bare, ""); !aerr.HasCode(err, aerr.ToolUnreachable) || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("git missing: %v", err)
	}
}

func TestCacheFolderErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := Cache{Dir: filepath.Join(file, "cache"), Git: testGit(t)}
	if _, _, err := c.Ensure("github.com/acme/users", "x", ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("cache under a file: %v", err)
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	c = Cache{Dir: filepath.Join(ro, "cache"), Git: testGit(t)}
	if _, _, err := c.Ensure("github.com/acme/users", "x", ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("cache that cannot be created: %v", err)
	}
}

// gitChecksum hashes every file under a .git folder, names and contents.
func gitChecksum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(filepath.Join(dir, ".git"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\n%x\n", p, sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestLocalGitChecksumsUnchanged(t *testing.T) {
	bare, dev := upstream(t)
	before := gitChecksum(t, dev)
	Read(dev)
	if _, _, err := (Cache{Dir: filepath.Join(t.TempDir(), "cache"), Git: testGit(t)}).Ensure("github.com/acme/users", bare, dev); err != nil {
		t.Fatal(err)
	}
	if after := gitChecksum(t, dev); after != before {
		t.Error("indexing or seeding the cache changed the developer's .git folder (FR-DISC-016)")
	}
}

func TestCloneURL(t *testing.T) {
	ssh := filepath.Join(t.TempDir(), "ssh")
	clone(t, ssh, "git@github.com:acme/users.git")
	token := filepath.Join(t.TempDir(), "token")
	clone(t, token, "https://x-access-token:ghs_secret@github.com/acme/users.git")
	tests := []struct{ local, want string }{
		{ssh, "git@github.com:acme/users.git"},
		{token, "https://github.com/acme/users"},
		{"", "https://github.com/acme/users"},
	}
	for _, tt := range tests {
		if got := CloneURL("github.com/acme/users", tt.local); got != tt.want {
			t.Errorf("CloneURL(%s) = %q, want %q", tt.local, got, tt.want)
		}
	}
	if got := CloneURL("github.com/acme/orders", ssh); got != "https://github.com/acme/orders" {
		t.Errorf("a clone of another repo = %q", got)
	}
}
