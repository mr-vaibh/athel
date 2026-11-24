package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/conventions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestBadTestPattern(t *testing.T) {
	files := []string{"src/main/java/com/acme/Users.java", "src/it/java/com/acme/UsersIT.java", "qa/flows/orders.check.ts",
		"src/latest/version.go", "web/orders.spec.ts", "tests/helpers.py"}
	tests := []struct {
		pattern, want string
	}{
		{"src/it/**", ""},
		{"**/*IT.java", ""},
		{"web/**/*.spec.ts", ""},
		{"tests/**", ""},
		{"qa/**", "names no test folder"},
		{"src/**", "names no test folder"},
		{"src/latest/**", "names no test folder"},
		{"**", "names no test folder"},
		{"../other/tests/**", "stays inside it"},
		{"/tests/**", "stays inside it"},
		{"e2e/**", "matches no file"},
	}
	for _, tt := range tests {
		got := badTestPattern(tt.pattern, files)
		if tt.want == "" && got != "" || tt.want != "" && !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func qaWrite(h *harness, file string) map[string]any {
	h.t.Helper()
	tree := filepath.Join(h.root, "repos", "users@CHG-001")
	raw, _ := json.Marshal(map[string]any{"file_path": filepath.Join(tree, file), "old_string": "package", "new_string": "package"})
	return h.mustOK("policy", "decide", "--agent", "athel:qa", "--tool", "Edit", "--input", string(raw), "--cwd", tree)
}

// TestQAScopeFromConventions (M5-plan G15): QA may write where the repo keeps its tests, once
// the repo's own pattern is saved; patterns that match no file are refused.
func TestQAScopeFromConventions(t *testing.T) {
	h, tree := goGates(t)
	if out, err := commit(t, h, tree, "src/e2e/flow_check.go", "package e2e\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if v := qaWrite(h, "src/e2e/flow_check.go"); v["decision"] != "deny" {
		t.Errorf("before the pattern, e2e/ is product code to QA: %v", v)
	}
	if code, _ := h.runJSON("conventions", "tests", "users"); code != aerr.ExitUsage {
		t.Errorf("no pattern: exit %d", code)
	}
	if code, env := h.runJSON("conventions", "tests", "users", "e2e/**"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "matches no file") {
		t.Errorf("a pattern for files the repo lacks: %v", env)
	}
	v := h.mustOK("conventions", "tests", "users", "src/*_test.go")
	if v["repo"] != "github.com/acme/users" {
		t.Errorf("saved: %v", v)
	}
	dir := filepath.Join(h.root, ".athel", "repos", conventions.Dir("github.com/acme/users"))
	doc := "schema: athel/test-patterns@1\nrepo: github.com/acme/users\npatterns: [\"src/e2e/**\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "tests.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := qaWrite(h, "src/e2e/flow_check.go"); v["decision"] != "allow" {
		t.Errorf("with the pattern, QA writes there: %v", v)
	}
	if v := qaWrite(h, "src/a.go"); v["decision"] != "deny" {
		t.Errorf("product code stays out of reach: %v", v)
	}
	if err := os.WriteFile(filepath.Join(dir, "tests.yaml"), []byte("schema: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := qaWrite(h, "src/e2e/flow_check.go"); v["decision"] != "deny" {
		t.Errorf("a broken tests.yaml falls back to the defaults: %v", v)
	}
}
