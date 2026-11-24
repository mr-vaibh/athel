package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestSetShow(t *testing.T) {
	h := setup(t)
	if code, env := h.runJSON("set", "show", "CHG-001"); code != aerr.ExitState || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("show without a plan: %d %v", code, env)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "start")
	m3Spec(t, h)
	m3Plan(t, h)
	v := h.mustOK("set", "show", "CHG-001")
	set, _ := v["set"].(map[string]any)
	if v["approved"] != false || set["strategy"] != "single" {
		t.Errorf("before approval, a preview from the plan: %v", v)
	}
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
	v = h.mustOK("set", "show", "CHG-001")
	set, _ = v["set"].(map[string]any)
	repos, _ := set["repos"].([]any)
	if v["approved"] != true || len(repos) != 1 || repos[0].(map[string]any)["role"] == "" {
		t.Errorf("after approval, the set fixed then: %v", v)
	}
}

func TestSetShowTwoWayPlan(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "start")
	twoWay := "schema: athel/plan@1\nchange: CHG-001\nhash: null\ntasks:\n" +
		"  - {id: T-01, title: a, repo: users, paths: [a.go], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: []}\n" +
		"  - {id: T-02, title: b, repo: orders, paths: [b.go], verbs: [edit], covers: [AC-1], depends_on: [T-01], size: S, gates: []}\n" +
		"  - {id: T-03, title: c, repo: users, paths: [c.go], verbs: [edit], covers: [AC-1], depends_on: [T-02], size: S, gates: []}\n"
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "plan.yaml", twoWay))
	code, env := h.runJSON("set", "show", "CHG-001")
	if code != aerr.ExitState || !strings.Contains(asJSON(t, env), "wait on each other") {
		t.Errorf("a two-way plan has no set: %d %v", code, env)
	}
}
