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

// TestPlanSetChecksTheChangeSet: the blast radius puts notifications in the change set as a
// consumer of a breaking change, so a plan must give it a task after users expands, and remove
// the old contract in a deferred task (M5-plan §5.2).
func TestPlanSetChecksTheChangeSet(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.stdin = strings.Replace(plannedBlast(), "in_change_set: false", "in_change_set: true", 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p.yaml", strings.Replace(m3PlanDoc, "repo: r,", "repo: users,", 1)))
	for _, want := range []string{
		"notifications consumes PUT /v1/users/{id} from users and is in the change set, but no task changes it",
		"PUT /v1/users/{id} is a breaking change for notifications; plan it as expand in users",
	} {
		if code != aerr.ExitState || !strings.Contains(asJSON(t, env), want) {
			t.Errorf("plan set without the consumer: %d, want %q in %v", code, want, env)
		}
	}
	good := `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: Add phone_number, repo: users, kind: expand, paths: [api/**], verbs: [edit, test], covers: [AC-1], depends_on: [], size: S, gates: [], rollback: revert}
  - {id: T-02, title: Read phone_number, repo: notifications, kind: consume, paths: [src/**], verbs: [edit, test], covers: [AC-1], depends_on: [T-01], size: S, gates: []}
  - {id: T-03, title: Remove phone, repo: users, kind: contract, deferred: true, paths: [api/**], verbs: [edit], covers: [AC-1], depends_on: [T-02], size: S, gates: [], rollback: revert}
`
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "good.yaml", good))
	sum := h.mustOK("plan", "summary", "CHG-001")
	order, _ := sum["merge_order"].([]any)
	if len(order) != 3 || order[0] != "1. users (provider)" || order[1] != "2. notifications (consumer; after users)" ||
		!strings.HasPrefix(order[2].(string), "later, a separate change: users") {
		t.Errorf("merge order: %v", sum["merge_order"])
	}
}

func TestPlanSetRefusesTwoWayRepos(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	twoWay := `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: a, repo: users, kind: internal, paths: [a.go], verbs: [edit, test], covers: [AC-1], depends_on: [], size: S, gates: []}
  - {id: T-02, title: b, repo: orders, kind: internal, paths: [b.go], verbs: [edit, test], covers: [AC-1], depends_on: [T-01], size: S, gates: []}
  - {id: T-03, title: c, repo: users, kind: internal, paths: [c.go], verbs: [edit, test], covers: [AC-1], depends_on: [T-02], size: S, gates: []}
`
	code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p.yaml", twoWay))
	if code != aerr.ExitState || !strings.Contains(asJSON(t, env), "users and orders wait on each other") || !strings.Contains(asJSON(t, env), "coordinated-release card") {
		t.Errorf("two-way plan: %d %v", code, env)
	}
}
