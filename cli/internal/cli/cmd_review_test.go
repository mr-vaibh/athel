package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func (h *harness) reviewRound(t *testing.T, sub string) map[string]any {
	t.Helper()
	h.stdin = sub
	defer func() { h.stdin = "" }()
	return h.mustOK("review", "save", "CHG-001", "T-01", "-")
}

const majorFinding = `verdict: changes_requested
new:
  - {severity: major, source: reviewer, location: "src/ac_test.go:5", rule: tests.ac-covered, finding: The test names no behaviour., required: Assert the AC-1 behaviour.}
`

// TestReviewRounds (FR-BUILD-007/008): a finding stays open until the reviewer verifies a fix
// that changed its file; the review gate passes only then.
func TestReviewRounds(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	r1 := h.reviewRound(t, majorFinding)
	if r1["round"] != float64(1) || !strings.Contains(asJSON(t, r1), `"id":"R1-01"`) {
		t.Fatalf("round 1: %v", r1)
	}
	h.runJSON("gate", "run", "--stage", "full", "CHG-001", "T-01")
	if m := lastResult(t, h, "review"); m["status"] != "fail" || !strings.Contains(asJSON(t, m), "R1-01 blocks") {
		t.Errorf("an open major blocks: %v", m)
	}
	if code, env := h.runJSON("review", "save", "CHG-001", "T-01", "-"); code == 0 {
		t.Errorf("round 2 must judge R1-01: %v", env)
	}
	if out, err := commit(t, h, tree, "src/other_test.go", "package src\n\nimport \"testing\"\n\n// AC-1\nfunc TestOther(t *testing.T) {}\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("review", "fix", "CHG-001", "T-01", "R1-01", "--commit", "HEAD")
	h.stdin = "verdict: approved\nprevious: [{id: R1-01, status: verified}]\n"
	if _, env := h.runJSON("review", "save", "CHG-001", "T-01", "-"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "does not change src/ac_test.go") {
		t.Errorf("a fix that changed another file cannot be verified: %v", env)
	}
	if out, err := commit(t, h, tree, "src/ac_test.go", "package src\n\nimport \"testing\"\n\n// TestAC1 checks AC-1: Pos.\nfunc TestAC1(t *testing.T) { t.Log(\"AC-1\") }\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("review", "fix", "CHG-001", "T-01", "R1-01", "--commit", "HEAD")
	r2 := h.reviewRound(t, "verdict: approved\nprevious: [{id: R1-01, status: verified, note: asserts the behaviour now}]\n")
	if r2["round"] != float64(2) || !strings.Contains(asJSON(t, r2), `"status":"verified"`) {
		t.Fatalf("round 2: %v", r2)
	}
	h.runJSON("gate", "run", "--stage", "full", "CHG-001", "T-01")
	if m := lastResult(t, h, "review"); m["status"] != "pass" {
		t.Errorf("verified and approved: %v", m)
	}
	if v := h.mustOK("review", "show", "CHG-001", "T-01"); v["round"] != float64(2) {
		t.Errorf("show: %v", v)
	}
}

func TestReviewDisputeTwiceRaisesCard(t *testing.T) {
	h, _ := goGates(t)
	h.reviewRound(t, majorFinding)
	h.mustOK("review", "dispute", "CHG-001", "T-01", "R1-01", "--reason", "the test is a placeholder by design")
	h.reviewRound(t, "verdict: changes_requested\nprevious: [{id: R1-01, status: open, note: placeholders hide missing behaviour}]\n")
	h.mustOK("review", "dispute", "CHG-001", "T-01", "R1-01", "--reason", "still by design")
	open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open"))
	for _, want := range []string{"disagree about R1-01", "Reviewer: placeholders hide missing behaviour", "Developer: still by design"} {
		if !strings.Contains(open, want) {
			t.Errorf("the card lacks %q: %s", want, open)
		}
	}
}

func TestReviewRoundLimit(t *testing.T) {
	h, _ := goGates(t)
	h.reviewRound(t, majorFinding)
	for range 4 {
		h.reviewRound(t, "verdict: changes_requested\nprevious: [{id: R1-01, status: open}]\n")
	}
	h.stdin = "verdict: changes_requested\nprevious: [{id: R1-01, status: open}]\n"
	if _, env := h.runJSON("review", "save", "CHG-001", "T-01", "-"); errCode(env) != string(aerr.BudgetRounds) {
		t.Errorf("a sixth round: %v", env)
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); !strings.Contains(open, "reached 5 review rounds") {
		t.Errorf("a card asks the developer: %s", open)
	}
}

func TestReviewCommandErrors(t *testing.T) {
	h, _ := goGates(t)
	for _, args := range [][]string{
		{"review", "save", "CHG-001", "T-01"}, {"review", "show", "CHG-001", "T-01"},
		{"review", "fix", "CHG-001", "T-01", "R1-01", "--commit", "HEAD"}, {"review", "dispute", "CHG-001", "T-01", "R1-01", "--reason", "x"},
	} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	h.reviewRound(t, majorFinding)
	if _, env := h.runJSON("review", "fix", "CHG-001", "T-01", "R1-01", "--commit", "nope"); errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("an unknown commit: %v", env)
	}
	h.stdin = "verdict: approved\nbogus: 1\n"
	if _, env := h.runJSON("review", "save", "CHG-001", "T-01", "-"); errCode(env) != string(aerr.StateSchema) || !strings.Contains(asJSON(t, env), "review template") {
		t.Errorf("a malformed review names the template: %v", env)
	}
	h.stdin = ""
	if code, out, _ := h.run("review", "template"); code != 0 || !strings.Contains(out, "verdict: changes_requested") {
		t.Errorf("template: %d %s", code, out)
	}
}

// TestIdenticalGateFailuresRaiseCard (FR-BUILD-010): three red runs that fail the same way stop
// the loop with a card.
func TestIdenticalGateFailuresRaiseCard(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/bad.go", "package src\nfunc  X(){}\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	for range 3 {
		h.runJSON("gate", "run", "CHG-001", "T-01")
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); !strings.Contains(open, "failed the same way three times") {
		t.Errorf("a card after three identical failures: %s", open)
	}
}
