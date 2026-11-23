package cli

import (
	"fmt"
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

// TestReviewRoundLimit (FR-BUILD-010): a fifth round that is not done raises the card at once;
// a sixth is refused until the developer allows more rounds, and then it is saved.
// A dispute card is built from free text; long arguments are clipped to the card's limit, so
// the dispute is still recorded and the card raised.
func TestReviewDisputeCardFitsItsSchema(t *testing.T) {
	h, _ := goGates(t)
	long := strings.Repeat("the spec says so ", 200)
	h.reviewRound(t, majorFinding)
	h.mustOK("review", "dispute", "CHG-001", "T-01", "R1-01", "--reason", long)
	h.reviewRound(t, "verdict: changes_requested\nprevious: [{id: R1-01, status: open, note: "+strings.Repeat("still wrong ", 200)+"}]\n")
	h.mustOK("review", "dispute", "CHG-001", "T-01", "R1-01", "--reason", long)
	open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open"))
	if !strings.Contains(open, "disagree about R1-01") || !strings.Contains(open, "Developer: the spec says so") {
		t.Errorf("the dispute card: %s", open)
	}
}

func TestClip(t *testing.T) {
	for _, tt := range []struct {
		in   string
		n    int
		want string
	}{{"short", 10, "short"}, {"exactly", 7, "exactly"}, {"héllo wörld", 5, "héll…"}} {
		if got := clip(tt.in, tt.n); got != tt.want {
			t.Errorf("clip(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestReviewRoundLimit(t *testing.T) {
	h, _ := goGates(t)
	again := "verdict: changes_requested\nprevious: [{id: R1-01, status: open}]\n"
	h.reviewRound(t, majorFinding)
	for range 3 {
		h.reviewRound(t, again)
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); strings.Contains(open, "review rounds") {
		t.Errorf("no card before the limit: %s", open)
	}
	h.reviewRound(t, again)
	open := h.mustOK("decide", "list", "CHG-001", "--open")
	text := asJSON(t, open)
	for _, want := range []string{"reached 5 review rounds", "R1-01", "/athel:waive"} {
		if !strings.Contains(text, want) {
			t.Errorf("the card at round 5 lacks %q: %s", want, text)
		}
	}
	h.stdin = again
	if _, env := h.runJSON("review", "save", "CHG-001", "T-01", "-"); errCode(env) != string(aerr.BudgetRounds) {
		t.Errorf("a sixth round: %v", env)
	}
	if n := strings.Count(asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")), "review rounds"); n != 1 {
		t.Errorf("one card, not %d", n)
	}
	card := open["decisions"].([]any)[0].(map[string]any)["id"].(string)
	h.humanPrompt("/athel:decide CHG-001 " + card + "=b")
	h.mustOK("decide", "answer", "CHG-001", card+"=b")
	h.reviewRound(t, again)
	if v := h.mustOK("review", "show", "CHG-001", "T-01"); v["round"] != float64(6) {
		t.Errorf("round 6 once more rounds are allowed: %v", v)
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

// TestIdenticalGateFailuresRaiseCard (FR-BUILD-010): three fix attempts (new heads) that fail the
// same way stop the loop with a card; running the gates again on one head is not an attempt.
func TestIdenticalGateFailuresRaiseCard(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/bad.go", "package src\nfunc  X(){}\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	for range 3 {
		h.runJSON("gate", "run", "CHG-001", "T-01")
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); strings.Contains(open, "failed the same way three times") {
		t.Fatalf("three runs on one head are one attempt: %s", open)
	}
	for i := range 2 {
		if out, err := commit(t, h, tree, fmt.Sprintf("src/note%d.go", i), "package src\n", goodMsg); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		h.runJSON("gate", "run", "CHG-001", "T-01")
	}
	if open := asJSON(t, h.mustOK("decide", "list", "CHG-001", "--open")); !strings.Contains(open, "failed the same way three times") {
		t.Errorf("a card after three identical failures: %s", open)
	}
}

// TestBuildBlocksOnlyOnACard: the build step ends blocked only on an open card the developer can
// answer; a finding the developer thinks is wrong goes to a dispute, never to a silent block.
func TestBuildBlocksOnlyOnACard(t *testing.T) {
	h, _ := goGates(t)
	h.mustOK("state", "enter", "CHG-001", "build", "--task", "T-01")
	again := "verdict: changes_requested\nprevious: [{id: R1-01, status: open}]\n"
	h.reviewRound(t, majorFinding)
	_, env := h.runJSON("state", "exit", "CHG-001", "build", "--result", "blocked", "--reason", "R1-01 needs a waiver")
	if errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "review dispute") {
		t.Errorf("a block with no card to answer: %v", env)
	}
	for range 4 {
		h.reviewRound(t, again)
	}
	h.mustOK("state", "exit", "CHG-001", "build", "--result", "blocked", "--reason", "the round-limit card")
}
