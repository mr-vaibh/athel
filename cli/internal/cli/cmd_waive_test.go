package cli

import (
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// redLint is a building change whose lint gate is red: src/bad.go makes the scripted linter
// report a new finding.
func redLint(t *testing.T) *harness {
	t.Helper()
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/bad.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); !strings.Contains(asJSON(t, env), "lint fail") {
		t.Fatalf("lint must be red: %v", env)
	}
	return h
}

func (h *harness) waive(subject, until, reason string) (int, map[string]any) {
	return h.runJSON("gate", "waive", "CHG-001", "T-01", subject, "--until", until, "--reason", reason)
}

// TestGateWaiver (FR-BUILD-014): only the developer's own prompt lets a gate through, with a
// reason and an expiry; the gate counts as passing until the end of that day.
func TestGateWaiver(t *testing.T) {
	h := redLint(t)
	if _, env := h.waive("lint", "2026-11-20", "legacy finding"); errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Fatalf("a waiver without the developer's prompt: %v", env)
	}
	h.humanPrompt("/athel:waive CHG-001 T-01 lint until=2026-11-20 'legacy finding'")
	if _, env := h.waive("lint", "2026-11-20", "another reason"); errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Errorf("the grant binds the reason: %v", env)
	}
	if code, env := h.waive("lint", "2026-11-20", "legacy finding"); code != 0 {
		t.Fatalf("waive: %v", env)
	}
	run := h.mustOK("gate", "run", "CHG-001", "T-01")
	if run["green"] != true || !strings.Contains(asJSON(t, run), `"status":"waived"`) {
		t.Errorf("a waived gate counts as passing: %v", run)
	}
	h.clock.Advance(20 * 24 * time.Hour) // 2026-11-21: the waiver ended with 2026-11-20
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); !strings.Contains(asJSON(t, env), "lint fail") {
		t.Errorf("an expired waiver stops counting: %v", env)
	}
	if body := h.mustOK("pr", "body", "CHG-001", "users")["body"].(string); !strings.Contains(body, "- T-01 lint until 2026-11-20 by human: legacy finding") {
		t.Errorf("the PR body lists the waiver:\n%s", body)
	}
}

func TestGateWaiverRefusals(t *testing.T) {
	h, _ := goGates(t)
	for _, tt := range []struct{ subject, until, reason, want string }{
		{"build", "2026-11-20", "x", "never waivable"},
		{"security-scan", "2026-11-20", "x", "security owner"},
		{"lint", "2026-10-30", "x", "in the past"},
		{"lint", "2026-12-15", "x", "more than 30 days"},
		{"lint", "soon", "x", "YYYY-MM-DD"},
		{"lint", "2026-11-20", " ", "needs a reason"},
		{"nightly", "2026-11-20", "x", "has no gate nightly"},
	} {
		if _, env := h.waive(tt.subject, tt.until, tt.reason); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), tt.want) {
			t.Errorf("%s until %s: %v", tt.subject, tt.until, env)
		}
	}
}

// TestFindingWaiver: a major review finding can be waived by the developer, never a blocker.
func TestFindingWaiver(t *testing.T) {
	h, _ := goGates(t)
	h.reviewRound(t, "verdict: approved\nnew:\n  - {severity: major, source: reviewer, location: main.go, rule: errors.typed, finding: f, required: q}\n"+
		"  - {severity: blocker, source: reviewer, location: main.go, rule: security.authz, finding: g, required: q}\n")
	h.humanPrompt("/athel:waive CHG-001 T-01 R1-02 until=2026-11-20 'accepted'")
	if _, env := h.waive("R1-02", "2026-11-20", "accepted"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "a blocker must be fixed") {
		t.Errorf("a blocker: %v", env)
	}
	h.humanPrompt("/athel:waive CHG-001 T-01 R1-01 until=2026-11-20 'accepted risk'")
	h.mustOK("gate", "waive", "CHG-001", "T-01", "R1-01", "--until", "2026-11-20", "--reason", "accepted risk")
	if show := asJSON(t, h.mustOK("review", "show", "CHG-001", "T-01")); !strings.Contains(show, `"status":"waived"`) {
		t.Errorf("the finding is waived in the round: %s", show)
	}
}
