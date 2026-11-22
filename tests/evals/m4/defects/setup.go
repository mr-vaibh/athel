package defects

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
	"github.com/mr-vaibh/athel/tests/internal/m3docs"
)

// Change and Task are what Prepare builds: the first change of a fresh workspace, one task.
const (
	Change = "CHG-001"
	Task   = "T-01"
	setup  = "eval-setup" // the session that prepares the change; it releases the lock at the end
)

// Worktree is the task's worktree, where the case's diff is committed.
func Worktree(sb *evalkit.Sandbox, c Case) string {
	return filepath.Join(sb.WS, "repos", c.Repo+"@"+Change)
}

// Manifest is the reviewer's manifest, relative to the workspace, as the build loop gives it.
const Manifest = ".athel/changes/" + Change + "/reviews/" + Task + "/context.yaml"

// Prepare takes a fresh sandbox, with the CLI alone, to what the build loop has when the
// checkers start: a change in building whose one task's diff is the case's files, committed, with
// the reviewer's manifest written. No model is called.
func Prepare(sb *evalkit.Sandbox, c Case) error {
	run := func(stdin string, args ...string) error {
		_, err := sb.Athel(stdin, append([]string{"--json", "--session", setup}, args...)...)
		return err
	}
	remote := "github.com/acme/" + c.Repo
	steps := []struct {
		stdin string
		args  []string
	}{
		{"", []string{"change", "new", "--title", c.ID, "--prd-text", "# PRD\n\n" + c.Requirement}},
		{"", []string{"state", "transition", Change, "--to", "investigating", "--by", "athel", "--reason", "eval setup"}},
		{m3docs.Review, []string{"prd", "review", "save", Change, "-"}},
		{conventions, []string{"conventions", "save", remote, "-", "--new"}},
		{spec(c, remote), []string{"spec", "save", Change, "-"}},
		{plan(c), []string{"plan", "set", Change, "-"}},
	}
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		steps = append(steps, struct {
			stdin string
			args  []string
		}{"", []string{"state", "transition", Change, "--to", to, "--by", "athel", "--reason", "eval setup"}})
	}
	for _, s := range steps {
		if err := run(s.stdin, s.args...); err != nil {
			return err
		}
	}
	// The developer approves the plan: the prompt goes through the real UserPromptSubmit hook.
	in, _ := json.Marshal(map[string]string{"session_id": setup, "prompt_id": "p-approve", "prompt": "/athel:approve " + Change})
	if err := run(string(in), "hook", "user-prompt-submit"); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"state", "transition", Change, "--to", "plan_approved", "--by", "human", "--reason", "eval setup"},
		{"git", "preflight", Change, c.Repo},
		{"state", "transition", Change, "--to", "building", "--by", "athel", "--reason", "preflight clear"},
		{"state", "enter", Change, "build", "--task", Task},
		{"gate", "baseline", Change, Task},
	} {
		if err := run("", args...); err != nil {
			return err
		}
	}
	if err := commitCase(sb, c); err != nil {
		return err
	}
	if err := run("", "review", "context", Change, Task); err != nil {
		return err
	}
	in, _ = json.Marshal(map[string]string{"session_id": setup, "reason": "other"})
	return run(string(in), "hook", "session-end")
}

// commitCase writes the case's files into the worktree and commits them as the developer would,
// through Athel's commit hooks.
func commitCase(sb *evalkit.Sandbox, c Case) error {
	tree := Worktree(sb, c)
	for _, name := range c.FileNames() {
		p := filepath.Join(tree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(c.Files[name]), 0o600); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		append([]string{"add", "--"}, c.FileNames()...),
		{"-c", "user.name=Dev", "-c", "user.email=dev@example.invalid", "commit", "-q", "-m", "feat(" + c.Repo + "): implement " + Task,
			"--trailer", "Athel-Change: " + Change, "--trailer", "Athel-Task: " + Task},
	} {
		cmd := exec.Command("git", args...) //nolint:gosec // git with the case's own fixed arguments and file names
		cmd.Dir = tree
		cmd.Env = []string{"PATH=" + sb.SessionPath(), "HOME=" + sb.Home, "ATHEL_SESSION_ID=" + setup}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %w: %s", args, err, out)
		}
	}
	return nil
}

const conventions = "conventions: |\n  # Conventions\n  Go with the standard library only. net/http handlers, table-driven tests with httptest,\n" +
	"  errors returned and wrapped with %w, gofmt and go vet clean.\nmap: |\n  # Map\n  One package per folder; main.go wires the handlers.\n"

func spec(c Case, remote string) string {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	return "schema: athel/spec@1\nchange: " + Change + "\nservice: " + c.Repo + "\nrepo: " + remote + "\nrequirements:\n" +
		"  - {id: REQ-1, prd: [PRD-1], text: " + q(c.Requirement) + ", decisions: []}\n" +
		"interfaces_changed: []\ndata_changes: []\ndesign_notes: []\nacceptance_criteria:\n" +
		"  - {id: AC-1, req: [REQ-1], given: " + q(c.Criterion.Given) + ", when: " + q(c.Criterion.When) + ", then: " + q(c.Criterion.Then) + "}\n" +
		"out_of_scope: []\nassumptions: []\n"
}

func plan(c Case) string {
	return strings.Replace(m3docs.Plan(c.Repo, c.Paths(), []string{"edit", "create", "test", "commit", "push"}),
		"gates: []}", "gates: [], rollback: revert the commit}", 1)
}
