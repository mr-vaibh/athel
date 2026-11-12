package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// m3Review is a PRD review of setup()'s one-heading PRD ("# PRD", indexed as PRD-1).
const m3Review = `schema: athel/prd-review@1
change: CHG-001
version: v1
checklist:
  goal: {status: present, prd: [PRD-1]}
  actors: {status: missing, prd: []}
  scope: {status: missing, prd: []}
  out_of_scope: {status: missing, prd: []}
  functional_requirements: {status: present, prd: [PRD-1]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: missing, prd: []}
  constraints: {status: missing, prd: []}
  dependencies: {status: missing, prd: []}
ambiguities: []
contradictions: []
assumptions: []
`

// m3PlanDoc covers the one criterion of m3Spec's spec, in repo r (the worktree name tests use).
const m3PlanDoc = `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: x, repo: r, kind: internal, paths: [src/**], verbs: [edit, test], covers: [AC-1], depends_on: [], size: S, gates: []}
`

// m3Spec gives setup()'s change what `specified` needs: a PRD review, conventions and a valid
// spec (M3 step guards, ADR 021).
func m3Spec(t *testing.T, h *harness) {
	t.Helper()
	h.stdin = m3Review
	h.mustOK("prd", "review", "save", "CHG-001", "-")
	h.stdin = convDoc
	h.mustOK("conventions", "save", "github.com/example/users", "-", "--new")
	h.stdin = strings.NewReplacer("prd: [PRD-1.1]", "prd: [PRD-1]", "decisions: [D-1]", "decisions: []").Replace(specBlock)
	h.mustOK("spec", "save", "CHG-001", "-")
	h.stdin = ""
}

// m3Plan adds what `planned` needs on top: a plan that passes the planning lints.
func m3Plan(t *testing.T, h *harness) {
	t.Helper()
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "m3plan.yaml", m3PlanDoc))
}

func TestStepGuards(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	code, env := h.runJSON("state", "transition", "CHG-001", "--to", "specified", "--by", "athel", "--reason", "r")
	if code != aerr.ExitState || errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "has no review yet") || !strings.Contains(asJSON(t, env), "no spec yet") {
		t.Fatalf("specified without the work: %v", env)
	}
	m3Spec(t, h)
	for _, to := range []string{"specified", "awaiting_decisions"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "planned", "--by", "athel", "--reason", "r"); code == 0 || !strings.Contains(asJSON(t, env), "no plan yet") {
		t.Errorf("planned without a plan: %v", env)
	}
	h.stdin = strings.Replace(cardYAML, "blocks: [T-03]", "blocks: [plan]", 1)
	h.mustOK("decide", "add", "CHG-001", "-")
	h.stdin = ""
	m3Plan(t, h)
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "planned", "--by", "athel", "--reason", "r"); code == 0 || !strings.Contains(asJSON(t, env), "D-1 is unanswered and blocks the plan step") {
		t.Errorf("planned with a blocking card: %v", env)
	}
	h.humanPrompt("/athel:decide CHG-001 D-1=b")
	h.mustOK("decide", "answer", "CHG-001", "D-1=b")
	tr := h.mustOK("trace", "CHG-001")
	if tr["complete"] != true || !strings.Contains(asJSON(t, tr["rows"]), `"tasks":["T-01"]`) {
		t.Errorf("trace: %v", tr)
	}
	if _, err := os.Stat(filepath.Join(h.root, ".athel", "changes", "CHG-001", "traceability.md")); err != nil {
		t.Error(err)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "planned", "--by", "athel", "--reason", "r")
}

func TestGuardsSeeStaleWork(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	h.clock.Advance(31 * 24 * 60 * 60 * 1e9)
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "specified", "--by", "athel", "--reason", "r"); code == 0 || !strings.Contains(asJSON(t, env), "users: the conventions of github.com/example/users are stale") {
		t.Errorf("stale conventions block specified: %v", env)
	}
	if code, env := h.runJSON("trace", "CHG-009"); code == 0 || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("trace of an unknown change: %v", env)
	}
}
