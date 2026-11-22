package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const qaReport = `cases:
  - {category: negative, file: src/b_test.go, name: TestBRejectsEmpty, covers: AC-1, result: fail}
suite: {command: go test ./..., result: fail}
gaps: [no test for a very long name]
findings:
  - {severity: major, location: "src/a.go:1", rule: errors.no-silent-catch, finding: An empty name is accepted., required: Reject it with a typed error.}
`

const securityReport = `checked: [input validation, secrets]
findings:
  - {severity: blocker, cwe: CWE-89, location: "src/a.go:1", rule: security.parameterised-queries, finding: The name is put into SQL text., required: Pass it as a parameter.}
`

func (h *harness) report(t *testing.T, kind, body string) map[string]any {
	t.Helper()
	h.stdin = body
	defer func() { h.stdin = "" }()
	return h.mustOK(kind, "save", "CHG-001", "T-01", "-")
}

// TestReportsJoinTheRound: QA's and security's findings join the task's review round whichever
// is saved first, so the review gate cannot pass while one is open.
func TestReportsJoinTheRound(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if err := os.WriteFile(filepath.Join(tree, "src", "b_test.go"), []byte("package src\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	qa := h.report(t, "qa", qaReport)
	if qa["n"] != float64(1) || !strings.Contains(asJSON(t, qa), `"id":"Q1-01"`) {
		t.Fatalf("qa-1: %v", qa)
	}
	h.runJSON("gate", "run", "--stage", "full", "CHG-001", "T-01")
	if m := lastResult(t, h, "review"); m["status"] != "fail" || !strings.Contains(asJSON(t, m), "not in a review round yet: Q1-01") {
		t.Errorf("a report before round 1 still blocks: %v", m)
	}
	r1 := h.reviewRound(t, "verdict: approved\n")
	if !strings.Contains(asJSON(t, r1), `"from":"Q1-01","history"`) || !strings.Contains(asJSON(t, r1), `"id":"R1-01"`) {
		t.Errorf("round 1 takes in Q1-01: %v", r1)
	}
	h.report(t, "security", securityReport)
	show := asJSON(t, h.mustOK("review", "show", "CHG-001", "T-01"))
	if !strings.Contains(show, `"id":"R1-02"`) || !strings.Contains(show, `"from":"S1-01"`) {
		t.Errorf("a security report after the round joins it at once: %s", show)
	}
	h.runJSON("gate", "run", "--stage", "full", "CHG-001", "T-01")
	if m := lastResult(t, h, "review"); m["status"] != "fail" || !strings.Contains(asJSON(t, m), "R1-02 blocks") {
		t.Errorf("an approved round with a security blocker does not pass: %v", m)
	}
}

func TestReportChecks(t *testing.T) {
	h, tree := goGates(t)
	if err := os.WriteFile(filepath.Join(tree, "src", "b_test.go"), []byte("package src\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ kind, body, want string }{
		{"qa", strings.Replace(qaReport, "src/b_test.go", "src/a.go", 1), "src/a.go is not a test file"},
		{"qa", strings.Replace(qaReport, "src/b_test.go", "src/a_test.go", 1), "not changed by this task"},
		{"qa", "cases: []\nsuite: {command: x, result: pass}\ngaps: []\nfindings: []\nextra: 1\n", "extra"},
		{"security", strings.Replace(securityReport, "cwe: CWE-89, ", "", 1), "CWE"},
		{"build", "commands_run: []\nnotes: x\n", "commands_run"},
	}
	for _, c := range cases {
		h.stdin = c.body
		if _, env := h.runJSON(c.kind, "save", "CHG-001", "T-01", "-"); !strings.Contains(asJSON(t, env), c.want) || errCode(env) == "" {
			t.Errorf("%s: want %q, got %v", c.kind, c.want, env)
		}
	}
	h.stdin = ""
	if code, _ := h.runJSON("qa", "save", "CHG-001", "T-01"); code != aerr.ExitUsage {
		t.Errorf("usage: %d", code)
	}
}

// TestBuildSaveFillsWhatGitKnows: the developer gives its commands and notes; the CLI adds the
// changed files and tests. The reviewer's manifest never names the result (FR-BUILD-006).
func TestBuildSaveFillsWhatGitKnows(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/c_test.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	res := asJSON(t, h.report(t, "build", "commands_run: [{command: go test ./..., result: pass}]\nnotes: I assumed names are ASCII.\n"))
	if !strings.Contains(res, `"files_changed":["src/c_test.go"]`) || !strings.Contains(res, `"tests_added":["src/c_test.go"]`) || !strings.Contains(res, `"n":1`) {
		t.Errorf("%s", res)
	}
	if again := h.report(t, "build", "commands_run: [{command: go vet ./..., result: fail}]\nnotes: \"\"\n"); again["n"] != float64(2) {
		t.Errorf("the next round's result: %v", again)
	}
	if ctx := asJSON(t, h.mustOK("review", "context", "CHG-001", "T-01")); strings.Contains(ctx, "result-") || strings.Contains(ctx, "ASCII") {
		t.Errorf("the reviewer sees the developer's notes: %s", ctx)
	}
}

func TestReportTemplates(t *testing.T) {
	h := newHarness(t)
	for kind, want := range map[string]string{"build": "commands_run:", "qa": "category: negative", "security": "cwe: CWE-89"} {
		code, out, _ := h.run(kind, "template")
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("%s template: %d %s", kind, code, out)
		}
	}
}
