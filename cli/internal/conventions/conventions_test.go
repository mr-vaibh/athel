package conventions

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func runGit(dir string, args ...string) (int, string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid"}
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), string(out), nil
	}
	return 0, string(out), err
}

// repo builds a small Go service as a bare clone, like Athel's cache.
func repo(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	files := map[string]string{
		"go.mod":                     "module example.com/users\n\ngo 1.27\n",
		".golangci.yml":              "linters:\n  enable: [errcheck]\n",
		".github/workflows/ci.yml":   "on: push\n",
		"api/user.go":                "package api\n",
		"api/user_test.go":           "package api\n",
		"internal/store/db/pg/pg.go": "package pg\n",
		"vendor/x/x.go":              "package x\n",
		"web/src/app.test.ts":        "test('x', () => {})\n",
		"docs/big.md":                strings.Repeat("x", 10),
	}
	for p, body := range files {
		full := filepath.Join(work, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "init"}} {
		if code, out, err := runGit(work, args...); code != 0 || err != nil {
			t.Fatalf("git %v: %d %s %v", args, code, out, err)
		}
	}
	if err := os.WriteFile(filepath.Join(work, "api", "user.go"), []byte("package api\n\nfunc F() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, err := runGit(work, "commit", "-q", "-am", "change user"); code != 0 || err != nil {
		t.Fatalf("commit: %d %s %v", code, out, err)
	}
	bare := filepath.Join(t.TempDir(), "users.git")
	if code, out, err := runGit(work, "clone", "-q", "--bare", work, bare); code != 0 || err != nil {
		t.Fatalf("clone: %d %s %v", code, out, err)
	}
	return bare
}

func TestTake(t *testing.T) {
	s, err := Take(runGit, "github.com/example/users", repo(t))
	if err != nil {
		t.Fatal(err)
	}
	var inputs []string
	for _, in := range s.Inputs {
		inputs = append(inputs, in.Kind+" "+in.Path)
		if len(in.Blob) < 40 || in.Content == "" {
			t.Errorf("input %s lacks its blob or content", in.Path)
		}
	}
	if strings.Join(inputs, "; ") != "ci .github/workflows/ci.yml; lint .golangci.yml; build go.mod" {
		t.Errorf("inputs: %v", inputs)
	}
	if !slices.Equal(s.Tests, []string{"api/user_test.go", "web/src/app.test.ts"}) {
		t.Errorf("tests: %v", s.Tests)
	}
	if !slices.Contains(s.Folders, "internal/store/db") || slices.Contains(s.Folders, "internal/store/db/pg") || slices.Contains(s.Folders, "vendor") {
		t.Errorf("folders (depth 3, no vendor): %v", s.Folders)
	}
	if len(s.Recent) == 0 || s.Recent[0] != "api/user.go" || slices.Contains(s.Recent, "vendor/x/x.go") {
		t.Errorf("recent (newest first, no vendor): %v", s.Recent)
	}
	if len(s.Commit) != 40 {
		t.Errorf("commit: %q", s.Commit)
	}
}

func TestTakeTruncatesAndFails(t *testing.T) {
	dir := repo(t)
	big := func(d string, args ...string) (int, string, error) {
		if args[0] == "show" {
			return 0, strings.Repeat("y", MaxContent+5), nil
		}
		return runGit(d, args...)
	}
	s, err := Take(big, "r", dir)
	if err != nil || !s.Inputs[0].Truncated || len(s.Inputs[0].Content) != MaxContent {
		t.Errorf("truncation: %v %v", s.Inputs, err)
	}
	for _, fail := range []string{"rev-parse", "ls-tree", "show", "log"} {
		failing := func(d string, args ...string) (int, string, error) {
			if args[0] == fail && (fail != "rev-parse" || args[1] == "HEAD") {
				return 128, "fatal: nope", nil
			}
			return runGit(d, args...)
		}
		if _, err := Take(failing, "r", dir); err == nil || !strings.Contains(err.Error(), "exited 128: fatal: nope") {
			t.Errorf("%s failing: %v", fail, err)
		}
	}
	broken := func(string, ...string) (int, string, error) { return 0, "", errors.New("no git") }
	if _, err := Take(broken, "r", dir); err == nil {
		t.Error("git missing must fail")
	}
	blobFails := func(d string, args ...string) (int, string, error) {
		if args[0] == "rev-parse" && args[1] != "HEAD" {
			return 128, "bad", nil
		}
		return runGit(d, args...)
	}
	if _, err := Take(blobFails, "r", dir); err == nil {
		t.Error("a blob lookup failing must fail")
	}
}

func TestStale(t *testing.T) {
	surveyed := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	m := Meta{Inputs: []Input{{Path: "go.mod", Blob: "aaa"}}}
	blobs := map[string]string{"go.mod": "aaa"}
	at := func(p string) (string, bool) { b, ok := blobs[p]; return b, ok }
	if why := Stale(m, surveyed.Add(29*24*time.Hour), surveyed, at); why != "" {
		t.Errorf("fresh: %s", why)
	}
	if why := Stale(m, surveyed.Add(31*24*time.Hour), surveyed, at); !strings.Contains(why, "30 days") {
		t.Errorf("old: %s", why)
	}
	blobs["go.mod"] = "bbb"
	if why := Stale(m, surveyed, surveyed, at); !strings.Contains(why, "go.mod changed") {
		t.Errorf("changed: %s", why)
	}
	delete(blobs, "go.mod")
	if why := Stale(m, surveyed, surveyed, at); !strings.Contains(why, "no longer exists") {
		t.Errorf("deleted: %s", why)
	}
	if Dir("github.com/acme/users") != "github.com-acme-users" {
		t.Error("dir")
	}
}

func TestRemote(t *testing.T) {
	norm := func(s string) string {
		switch s {
		case "git@github.com:acme/users.git", "https://github.com/acme/users":
			return "github.com/acme/users"
		}
		return ""
	}
	for in, want := range map[string]string{"git@github.com:acme/users.git": "github.com/acme/users", "github.com/acme/users": "github.com/acme/users",
		"ftp://x/y/z": "ftp://x/y/z", "billing-v2": "billing-v2"} {
		if got := Remote(in, norm); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
