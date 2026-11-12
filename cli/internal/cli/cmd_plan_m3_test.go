package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const planV2Doc = `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: Store the new phone number, repo: users, kind: internal, paths: ["api/**"], verbs: [edit, test, commit], covers: [AC-1], depends_on: [], size: S, gates: [build, unit]}
`

// planSetup is a change with a saved users spec (REQ-1, AC-1) and one open card (D-1).
func planSetup(t *testing.T) *harness {
	t.Helper()
	h := specSetup(t)
	h.stdin = specBlock
	h.mustOK("spec", "save", "CHG-001", "-")
	h.stdin = ""
	return h
}

func TestPlanSetLintsOnceSpecsExist(t *testing.T) {
	h := planSetup(t)
	v1 := strings.NewReplacer("plan@2", "plan@1", "kind: internal, ", "").Replace(planV2Doc)
	if code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p1.yaml", v1)); code == 0 || errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "must be athel/plan@2") {
		t.Errorf("plan@1 after specs: %v", env)
	}
	uncovered := strings.Replace(planV2Doc, "covers: [AC-1]", "covers: [AC-7]", 1)
	if code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p2.yaml", uncovered)); code == 0 || !strings.Contains(asJSON(t, env), "AC-1 is covered by no task") {
		t.Errorf("coverage: %v", env)
	}
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "p3.yaml", planV2Doc))
	s := h.mustOK("plan", "summary", "CHG-001")
	if !strings.Contains(asJSON(t, s["repos"]), "T-01 Store the new phone number") || !strings.Contains(asJSON(t, s["open_assumptions"]), "D-1 unanswered") ||
		s["estimated_hours"] != 0.5 || s["risk"] != "" {
		t.Errorf("summary: %v", s)
	}
	changed := strings.Replace(planV2Doc, "size: S", "size: M", 1)
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "p4.yaml", changed))
	if d := h.mustOK("plan", "diff", "CHG-001", "--against", writeTemp(t, "p5.yaml", planV2Doc)); asJSON(t, d) != `{"added":[],"changed":["T-01"],"removed":[]}` {
		t.Errorf("diff: %v", d)
	}
}

func TestPlanRollbackFollowsBlastRisk(t *testing.T) {
	h := planSetup(t)
	h.stdin = strings.Replace(blastDoc, "ask: ASK-001", "change: CHG-001", 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	expand := strings.Replace(planV2Doc, "kind: internal", "kind: expand", 1)
	if code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p.yaml", expand)); code == 0 || !strings.Contains(asJSON(t, env), "T-01 (expand) needs a rollback at CRITICAL risk") {
		t.Errorf("CRITICAL without rollback: %v", env)
	}
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "q.yaml", strings.Replace(expand, "gates: [build, unit]}", "gates: [build, unit], rollback: flag phone_v2 off}", 1)))
	if s := h.mustOK("plan", "summary", "CHG-001"); s["risk"] != "CRITICAL" {
		t.Errorf("summary risk: %v", s)
	}
}

func TestPlanM3Errors(t *testing.T) {
	h := planSetup(t)
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{"plan", "diff", "CHG-001"}, aerr.ExitUsage},
		{[]string{"plan", "diff", "CHG-001", "--against", "/nonexistent.yaml"}, aerr.ExitState},
		{[]string{"plan", "diff", "CHG-001", "--against", writeTemp(t, "bad.yaml", "schema: x\n")}, aerr.ExitState},
		{[]string{"plan", "diff", "CHG-001", "--against", writeTemp(t, "ok.yaml", planV2Doc)}, aerr.ExitState}, // no plan set yet
		{[]string{"plan", "summary", "CHG-001"}, aerr.ExitState},
		{[]string{"plan", "set", "CHG-001", writeTemp(t, "bad2.yaml", "schema: athel/plan@2\n")}, aerr.ExitState},
	} {
		if code, env := h.runJSON(tt.args...); code != tt.code {
			t.Errorf("%v: exit %d, want %d (%v)", tt.args, code, tt.code, env)
		}
	}
}
