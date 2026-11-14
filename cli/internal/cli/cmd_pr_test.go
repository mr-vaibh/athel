package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// recordingGH puts a gh on PATH that logs every call to the returned file, prints a PR URL for
// pr create and an empty list for pr list.
func recordingGH(t *testing.T, h *harness) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\nprintf '%s' \"$*\" | tr '\\n' '~' >> '" + log + "'\necho >> '" + log + "'\ncase \"$1 $2\" in\n" +
		"  'pr create') echo 'https://github.com/acme/users/pull/901' ;;\n  'pr list') echo '[]' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	return log
}

func calls(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func TestPROpenAndUpdate(t *testing.T) {
	h, _, _, _ := building(t)
	log := recordingGH(t, h)
	if _, env := h.runJSON("pr", "open", "CHG-001", "users"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "not pushed yet") {
		t.Errorf("a PR before the push: %v", env)
	}
	if _, env := h.runJSON("pr", "update", "CHG-001", "users"); errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("an update before the PR: %v", env)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	v := h.mustOK("pr", "open", "CHG-001", "users")
	if v["url"] != "https://github.com/acme/users/pull/901" || v["created"] != true || v["title"] != "feat(users): phone update" {
		t.Fatalf("open = %v", v)
	}
	got := calls(t, log)
	create := got[len(got)-1]
	for _, want := range []string{"pr create --repo acme/users --head athel/CHG-001-phone-update --base main", "--title feat(users): phone update", "## Summary", "--draft"} {
		if !strings.Contains(create, want) {
			t.Errorf("gh pr create lacks %q: %s", want, create)
		}
	}
	if again := h.mustOK("pr", "open", "CHG-001", "users"); again["created"] != false || len(calls(t, log)) != len(got) {
		t.Errorf("a second open returns the PR without creating another: %v", again)
	}
	if repos := h.mustOK("state", "show", "CHG-001")["repos"].([]any); repos[0].(map[string]any)["pr"] != "https://github.com/acme/users/pull/901" {
		t.Errorf("PR not recorded: %v", repos)
	}
	h.mustOK("pr", "update", "CHG-001", "users")
	if last := calls(t, log); !strings.HasPrefix(last[len(last)-1], "pr edit 901 --repo acme/users --title feat(users): phone update --body ## Summary") {
		t.Errorf("update = %s", last[len(last)-1])
	}
	for _, args := range [][]string{{"pr", "merge", "CHG-001", "users"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("there is no %v", args)
		}
	}
}

func TestPRBody(t *testing.T) {
	h, _, _, _ := building(t)
	v := h.mustOK("pr", "body", "CHG-001", "users")
	body := v["body"].(string)
	for _, want := range []string{"## Summary", "Phone update (CHG-001) in users", "## What changed", "- `src/` (1 file, 1 line)", "## Linked PRs and merge order", "T-01: revert the commit"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if v["lines"] != float64(1) || v["max_changed_lines"] != float64(400) {
		t.Errorf("size = %v", v)
	}
	for _, args := range [][]string{{"pr", "body", "CHG-001"}, {"pr", "body", "CHG-009", "users"}, {"pr", "body", "CHG-001", "orders"}} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
}

// TestPROversizedProposesSplit (FR-GIT-009): a diff over the per-PR limit is refused before
// anything reaches GitHub, with the plan's tasks grouped into PRs under the limit.
func TestPROversizedProposesSplit(t *testing.T) {
	h, tree, _, _ := building(t)
	log := recordingGH(t, h)
	if out, err := commit(t, h, tree, "src/more.go", "package src\n\nconst (\n\ta = 1\n\tb = 2\n)\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	before := len(calls(t, log))
	override := "schema: athel/git@1\npr:\n  max_changed_lines: 2\n"
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "git.yaml"), []byte(override), 0o600); err != nil {
		t.Fatal(err)
	}
	_, env := h.runJSON("pr", "open", "CHG-001", "users")
	if errCode(env) != string(aerr.GatePRSize) || !strings.Contains(asJSON(t, env), "the diff is 7 changed lines; git.yaml allows 2 per PR. Proposed split: PR 1: T-01 (7 lines)") {
		t.Errorf("oversized: %v", env)
	}
	if len(calls(t, log)) != before {
		t.Error("nothing reaches GitHub")
	}
}

func TestPRBodyListsDecisions(t *testing.T) {
	h, _, _, _ := building(t)
	h.stdin = "step: build\nquestion: Which log level for the new path?\nchoice: info\nreason: matches the existing handlers\n"
	h.mustOK("decide", "record", "CHG-001", "-")
	h.stdin = ""
	body := h.mustOK("pr", "body", "CHG-001", "users")["body"].(string)
	if !strings.Contains(body, "Which log level for the new path? → info (athel)") {
		t.Errorf("Athel's own decision is in the body:\n%s", body)
	}
}

func TestPRToolFailures(t *testing.T) {
	h, _, _, _ := building(t)
	h.mustOK("git", "push", "CHG-001", "users")
	failing := func(script string) {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
		h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	}
	failing("echo 'HTTP 403' >&2\nexit 1\n")
	if _, env := h.runJSON("pr", "open", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) {
		t.Errorf("gh failing: %v", env)
	}
	failing("exit 0\n")
	if _, env := h.runJSON("pr", "open", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) || !strings.Contains(asJSON(t, env), "printed no URL") {
		t.Errorf("gh printing nothing: %v", env)
	}
	if repos := h.mustOK("state", "show", "CHG-001")["repos"].([]any); repos[0].(map[string]any)["pr"] != nil {
		t.Errorf("no PR recorded after a failure: %v", repos)
	}
	recordingGH(t, h)
	h.mustOK("pr", "open", "CHG-001", "users")
	failing("echo 'HTTP 404' >&2\nexit 1\n")
	if _, env := h.runJSON("pr", "update", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) {
		t.Errorf("gh failing on edit: %v", env)
	}
}
