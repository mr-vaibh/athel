package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/pr"
)

// recordingGH puts a gh on PATH that logs every call to the returned file, prints a PR URL for
// pr create and an empty list for pr list.
func recordingGH(t *testing.T, h *harness) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\nprintf '%s' \"$*\" | tr '\\n' '~' >> '" + log + "'\necho >> '" + log + "'\ncase \"$1 $2\" in\n" +
		"  'pr create') echo 'https://github.com/acme/users/pull/901' ;;\n  'pr list') echo '[]' ;;\n  'label list') echo '[]' ;;\nesac\n"
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
	labelled(t, got)
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
	// ADR 028: the refusal is a card for the developer, never a dead end. One task alone is over
	// the limit, so raising it is recommended; once answered, the PR opens.
	open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open"))
	if !strings.Contains(open, "The users PR is over git.yaml's limit of 2 changed lines") || !strings.Contains(open, "Allow up to 50 changed lines in this PR") ||
		!strings.Contains(open, `"recommendation":{"option":"a"`) || !strings.Contains(asJSON(t, env), "answers card D-") {
		t.Fatalf("size card: %s %v", open, env)
	}
	if _, again := h.runJSON("pr", "open", "CHG-001", "users"); errCode(again) != string(aerr.GatePRSize) ||
		strings.Count(asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")), "over git.yaml's limit") != 1 {
		t.Errorf("a second refusal asks once: %v", again)
	}
	id := regexp.MustCompile(`"id":"(D-[0-9]+)"`).FindStringSubmatch(open)[1]
	h.humanPrompt("/athel:decide CHG-001 " + id + "=a")
	h.mustOK("decide", "answer", "CHG-001", id+"=a")
	h.mustOK("pr", "open", "CHG-001", "users")
	if len(calls(t, log)) == before {
		t.Error("the PR opens within the allowed size")
	}
}

// TestSizeCardRecommendsRaisingTheLimit (ADR 032): Athel opens one PR per repo, so even when the
// plan's tasks would split under the limit, raising it is recommended; re-planning stays the safe
// default and names the split.
func TestSizeCardRecommendsRaisingTheLimit(t *testing.T) {
	d := sizeCard("users", 500, 400, []pr.Group{{Tasks: []string{"T-01"}, Lines: 300}, {Tasks: []string{"T-02"}, Lines: 200}})
	if d.Recommendation.Option != "a" || d.Options[0].Label != "Allow up to 650 changed lines in this PR" || d.Default != "b" {
		t.Errorf("%+v", d)
	}
	if !strings.Contains(d.Options[1].Consequence, "follow-up change") || !strings.Contains(d.Recommendation.Reason, "one PR per repo") {
		t.Errorf("option b and the reason say what re-planning means: %+v", d)
	}
}

func TestPRBodyListsDecisions(t *testing.T) {
	h, _, _, _ := building(t)
	h.stdin = "step: build\nquestion: Which log level for the new path?\nchoice: info\nreason: matches the existing handlers\n"
	h.mustOK("decide", "record", "CHG-001", "-")
	h.stdin = cardYAML
	card := h.mustOK("decide", "add", "CHG-001", "-")["id"].(string)
	h.stdin = ""
	h.mustOK("decide", "present", "CHG-001", "--step", "spec")
	h.humanPrompt("/athel:decide CHG-001 " + card + "=a")
	h.mustOK("decide", "answer", "CHG-001", card+"=a")
	body := h.mustOK("pr", "body", "CHG-001", "users")["body"].(string)
	for _, want := range []string{"Which log level for the new path? → info (athel)", card + " Which phone formats should the API accept? → a (human)"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lacks %q:\n%s", want, body)
		}
	}
}

// TestPRBodyListsOpenMinors (doc 06 severity table): a minor finding left open never blocks the
// task, so the PR lists it for the human reviewer.
func TestPRBodyListsOpenMinors(t *testing.T) {
	h, _, _, _ := building(t)
	h.reviewRound(t, "verdict: approved\nnew:\n  - {severity: minor, source: security, location: \"src/a.go:1\", rule: security.input-validation, finding: Format characters pass., required: Reject them.}\n")
	body := h.mustOK("pr", "body", "CHG-001", "users")["body"].(string)
	if !strings.Contains(body, "- T-01 R1-01 (minor, open) src/a.go:1: Format characters pass.") {
		t.Errorf("the body lacks the open minor:\n%s", body)
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
	if _, env := h.runJSON("pr", "open", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) || !strings.Contains(asJSON(t, env), "read gh label list") {
		t.Errorf("gh printing no labels: %v", env)
	}
	failing("case \"$1 $2\" in 'label list') echo '[{\"name\":\"athel:CHG-001\"}]' ;; esac\nexit 0\n")
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

// TestSizeCardReplanGoesBackToPlanning (ADR 032): answering "re-plan" takes the change back to
// planned, where the architect moves part of the work to a follow-up change.
func TestSizeCardReplanGoesBackToPlanning(t *testing.T) {
	h, tree, _, _ := building(t)
	recordingGH(t, h)
	if out, err := commit(t, h, tree, "src/more.go", "package src\n\nconst (\n\ta = 1\n\tb = 2\n)\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "git.yaml"), []byte("schema: athel/git@1\npr:\n  max_changed_lines: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.runJSON("pr", "open", "CHG-001", "users")
	open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open"))
	if !strings.Contains(open, "Re-plan this change with a smaller scope") {
		t.Fatalf("option b: %s", open)
	}
	id := regexp.MustCompile(`"id":"(D-[0-9]+)"`).FindStringSubmatch(open)[1]
	h.humanPrompt("/athel:decide CHG-001 " + id + "=b")
	h.mustOK("decide", "answer", "CHG-001", id+"=b")
	if v := h.mustOK("state", "transition", "CHG-001", "--to", "planned", "--by", "athel", "--reason", id+": re-plan with a smaller scope"); v["state"] != "planned" {
		t.Errorf("back to planning: %v", v)
	}
}

// labelled: the change label is made in a repo that lacks it, and the PR is opened with it
// (doc 04 §Cross-repo PR rules; M5-plan G6).
func labelled(t *testing.T, calls []string) {
	t.Helper()
	all := strings.Join(calls, "\n")
	if !strings.Contains(all, "label create athel:CHG-001 --repo acme/users") || !strings.Contains(calls[len(calls)-1], "--label athel:CHG-001") {
		t.Errorf("the change label: %v", calls)
	}
}
