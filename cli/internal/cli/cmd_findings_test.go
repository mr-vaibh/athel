package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const sideFinding = `found_by: reviewer
found_in: {step: build, task: T-01}
location: users/src/address.go:88
category: bug
severity: medium
effort: S
fix_risk: low
evidence: Postcode trimmed before validation only on create, not update.
suggested_fix: Apply the same trim in the update path; add a test.
`

// TestSideFindings (FR-FIND-001/003): any agent records a finding; only the developer's own
// /athel:triage chooses what happens to it; the PR body lists them with the choice.
func TestSideFindings(t *testing.T) {
	h, _ := goGates(t)
	h.stdin = sideFinding
	if v := h.mustOK("finding", "add", "CHG-001", "-"); v["id"] != "SF-1" {
		t.Fatalf("add: %v", v)
	}
	h.stdin = strings.Replace(strings.Replace(sideFinding, "category: bug", "category: security", 1), "severity: medium", "severity: high", 1)
	h.mustOK("finding", "add", "CHG-001", "-")
	h.stdin = "category: vibes\n"
	if _, env := h.runJSON("finding", "add", "CHG-001", "-"); errCode(env) == "" {
		t.Errorf("an invalid finding: %v", env)
	}
	h.stdin = ""
	list := h.mustOK("finding", "list", "CHG-001")
	if list["summary"] != "Found 2 issues outside this change. 2 quick wins: SF-2, SF-1. Fix them as separate PRs, file tickets, or ignore?" ||
		list["findings"].([]any)[0].(map[string]any)["id"] != "SF-2" {
		t.Errorf("security first, with the summary: %v", list)
	}
	if _, env := h.runJSON("finding", "triage", "CHG-001", "SF-1=ticket"); errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Errorf("without the developer's prompt: %v", env)
	}
	h.humanPrompt("/athel:triage CHG-001 SF-1=ticket")
	h.mustOK("finding", "triage", "CHG-001", "SF-1=ticket")
	h.humanPrompt("/athel:triage CHG-001 all=fix_pr")
	h.mustOK("finding", "triage", "CHG-001", "all=fix_pr")
	body := h.mustOK("pr", "body", "CHG-001", "users")["body"].(string)
	for _, want := range []string{"SF-2 (security, high)", "→ fix_pr", "SF-1 (bug, medium) users/src/address.go:88: Postcode trimmed before validation only on create, not update. → ticket"} {
		if !strings.Contains(body, want) {
			t.Errorf("the PR body lacks %q:\n%s", want, body)
		}
	}
	for _, args := range [][]string{{"finding", "triage", "CHG-001"}, {"finding", "triage", "CHG-001", "SF-1=later"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: %d", args, code)
		}
	}
	h.humanPrompt("/athel:triage CHG-001 SF-9=ignore")
	if _, env := h.runJSON("finding", "triage", "CHG-001", "SF-9=ignore"); errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("an unknown finding: %v", env)
	}
}
