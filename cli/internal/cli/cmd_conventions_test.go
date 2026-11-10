package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// pushUpstream commits a file to the fixture's bare "GitHub" remote through a scratch clone.
func pushUpstream(t *testing.T, git func(string, ...string), bare, name, body string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "w")
	git(t.TempDir(), "clone", "-q", bare, work)
	if err := os.WriteFile(filepath.Join(work, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	git(work, "add", name)
	git(work, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-qm", "update "+name)
	git(work, "push", "-q", "origin", "HEAD")
}

func TestSurveyAndConventions(t *testing.T) {
	h, _, src, git := cacheFixture(t)
	bare := filepath.Join(src, "users.git")
	pushUpstream(t, git, bare, "go.mod", "module example.com/users\n\ngo 1.27\n")
	s := h.mustOK("repos", "survey", "users")
	if s["repo"] != "github.com/acme/users" || len(s["commit"].(string)) != 40 || !strings.Contains(asJSON(t, s["inputs"]), `"path":"go.mod"`) {
		t.Fatalf("survey: %v", s)
	}
	h.stdin = convDoc
	saved := h.mustOK("conventions", "save", "github.com/acme/users", "-")
	for _, f := range []string{"conventions.md", "map.md", "conventions.yaml", "survey.json"} {
		if _, err := os.Stat(filepath.Join(saved["dir"].(string), f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if c := h.mustOK("conventions", "show", "users"); c["fresh"] != true {
		t.Errorf("fresh: %v", c)
	}
	pushUpstream(t, git, bare, "go.mod", "module example.com/users\n\ngo 1.28\n")
	h.mustOK("repos", "cache", "users") // the cache fetches the change
	if c := h.mustOK("conventions", "show", "users"); c["fresh"] != false || !strings.Contains(c["stale"].(string), "go.mod changed") {
		t.Errorf("after go.mod changed upstream: %v", c)
	}
}

func TestConventionsErrors(t *testing.T) {
	h := prdSetup(t)
	tests := []struct {
		name, stdin string
		args        []string
		code        aerr.Code
	}{
		{"no survey yet", convDoc, []string{"save", "github.com/example/users", "-"}, aerr.StateNotFound},
		{"empty map", "conventions: x\nmap: ''\n", []string{"save", "x", "-", "--new"}, aerr.StateInvalid},
		{"unknown field", convDoc + "extra: 1\n", []string{"save", "x", "-", "--new"}, aerr.StateSchema},
		{"missing file", "", []string{"save", "x", filepath.Join(h.root, "nope.yaml"), "--new"}, aerr.StateIO},
		{"nothing saved", "", []string{"show", "github.com/example/none"}, aerr.StateNotFound},
	}
	for _, tt := range tests {
		h.stdin = tt.stdin
		if code, env := h.runJSON(append([]string{"conventions"}, tt.args...)...); code == 0 || errCode(env) != string(tt.code) {
			t.Errorf("%s: %v", tt.name, env)
		}
	}
	h.stdin = convDoc
	h.mustOK("conventions", "save", "github.com/example/users", "-", "--new")
	dir := filepath.Join(h.root, ".athel", "repos", "github.com-example-users")
	if err := os.WriteFile(filepath.Join(dir, "conventions.yaml"), []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("conventions", "show", "github.com/example/users"); code == 0 || errCode(env) != string(aerr.StateSchema) {
		t.Errorf("corrupt record: %v", env)
	}
	if err := os.WriteFile(filepath.Join(dir, "survey.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.stdin = convDoc
	if code, env := h.runJSON("conventions", "save", "github.com/example/users", "-"); code == 0 || errCode(env) != string(aerr.StateSchema) {
		t.Errorf("corrupt survey: %v", env)
	}
	if code, env := h.runJSON("repos", "survey", "nowhere"); code == 0 {
		t.Errorf("survey of an unknown repo: %v", env)
	}
}
