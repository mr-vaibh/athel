package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// plannedBlast is blastDoc as a planned report on CHG-001, linked to the m3Spec spec (REQ-1).
func plannedBlast() string {
	return strings.NewReplacer("ask: ASK-001", "change: CHG-001", "mode: adhoc", "mode: planned",
		"evidence: [{kind: code, ref: users/api.go:12}]", "evidence: [{kind: spec, ref: REQ-1}, {kind: code, ref: users/api.go:12}]",
		"not_affected: []", "not_affected: [{item: orders, why: no reads of phone, evidence: [{kind: code, ref: orders/clients}]}]").Replace(blastDoc)
}

func TestPlannedBlastNeedsSpecLinks(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.stdin = strings.Replace(plannedBlast(), "{kind: spec, ref: REQ-1}, ", "", 1)
	if code, env := h.runJSON("blast", "save", "--change", "CHG-001", "-"); code == 0 || !strings.Contains(asJSON(t, env), "cites no requirement of the spec") {
		t.Errorf("planned without spec links: %v", env)
	}
	h.stdin = plannedBlast()
	if v := h.mustOK("blast", "save", "--change", "CHG-001", "-"); v["risk"] != "CRITICAL" {
		t.Errorf("saved: %v", v)
	}
}

func TestCriticalNeedsSignoff(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.stdin = plannedBlast()
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "p.yaml", m3PlanDoc))
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok"); code == 0 || !strings.Contains(asJSON(t, env), "CRITICAL and the current report has no sign-off") {
		t.Fatalf("approval before sign-off: %v", env)
	}
	if code, env := h.runJSON("blast", "signoff", "CHG-001"); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Errorf("sign-off without the developer: %v", env)
	}
	h.humanPrompt("/athel:signoff CHG-001")
	if so := h.mustOK("blast", "signoff", "CHG-001"); so["risk"] != "CRITICAL" || so["by"] != "human" {
		t.Errorf("signoff: %v", so)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
}

func TestSignoffVoidedByAChangedReport(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.stdin = plannedBlast()
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.humanPrompt("/athel:signoff CHG-001")
	h.mustOK("blast", "signoff", "CHG-001")
	h.stdin = strings.Replace(plannedBlast(), "Rename phone to phone_number", "Rename phone to phone_e164", 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "p.yaml", m3PlanDoc))
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok"); code == 0 || !strings.Contains(asJSON(t, env), "no sign-off") {
		t.Errorf("a changed report keeps no sign-off: %v", env)
	}
	if code, env := h.runJSON("blast", "signoff", "CHG-009"); code == 0 || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("unknown change: %v", env)
	}
}
