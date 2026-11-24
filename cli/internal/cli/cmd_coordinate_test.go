package cli

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// internalPlan changes users and notifications with no expand path: right only for a
// coordinated release.
const internalPlan = `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: Rename phone, repo: users, kind: internal, paths: [api/**], verbs: [edit, test], covers: [AC-1], depends_on: [], size: S, gates: [], rollback: revert}
  - {id: T-02, title: Read the new name, repo: notifications, kind: internal, paths: [src/**], verbs: [edit, test], covers: [AC-1], depends_on: [T-01], size: S, gates: [], rollback: revert}
`

func TestCoordinatedCardSetsStrategy(t *testing.T) {
	h := setup(t)
	m3Spec(t, h)
	h.stdin = strings.Replace(plannedBlast(), "in_change_set: false", "in_change_set: true", 1)
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	if code, env := h.runJSON("plan", "set", "CHG-001", writeTemp(t, "p.yaml", internalPlan)); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "raise a coordinated-release card") {
		t.Fatalf("a breaking change without an expand path or a card: %v", env)
	}
	v := h.mustOK("set", "coordinate", "CHG-001", "--reason", "legal requires the old field gone in this release")
	again := h.mustOK("set", "coordinate", "CHG-001", "--reason", "legal requires the old field gone in this release")
	if v["card"] != "D-1" || again["card"] != "D-1" {
		t.Errorf("one card, raised once: %v %v", v, again)
	}
	card := h.mustOK("decide", "show", "CHG-001", "D-1")
	text := asJSON(t, card)
	for _, want := range []string{"Backward compatibility is not possible for PUT /v1/users/{id}", "used by notifications", "legal requires the old field gone", `"rule":"A2"`} {
		if !strings.Contains(text, want) {
			t.Errorf("card lacks %q: %s", want, text)
		}
	}
	h.humanPrompt("/athel:decide CHG-001 D-1=a")
	h.mustOK("decide", "answer", "CHG-001", "D-1=a")
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "p.yaml", internalPlan))
	if set := h.mustOK("set", "show", "CHG-001")["set"].(map[string]any); set["strategy"] != "coordinated" {
		t.Errorf("preview strategy: %v", set)
	}
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:signoff CHG-001")
	h.mustOK("blast", "signoff", "CHG-001")
	h.humanPrompt("/athel:approve CHG-001")
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
	if set := h.mustOK("set", "show", "CHG-001"); set["approved"] != true || set["set"].(map[string]any)["strategy"] != "coordinated" {
		t.Errorf("approved strategy: %v", set)
	}
}

func TestSetCoordinateRefusals(t *testing.T) {
	h := setup(t)
	if code, _ := h.runJSON("set", "coordinate", "CHG-001"); code != aerr.ExitUsage {
		t.Errorf("no reason: exit %d", code)
	}
	m3Spec(t, h)
	if code, env := h.runJSON("set", "coordinate", "CHG-001", "--reason", "x"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "nothing to coordinate") {
		t.Errorf("no blast radius: %v", env)
	}
	h.stdin = plannedBlast() // breaking, but its consumer is outside the change set
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	if code, env := h.runJSON("set", "coordinate", "CHG-001", "--reason", "x"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "nothing to coordinate") {
		t.Errorf("no consumer in the set: %v", env)
	}
	if code, _ := h.runJSON("set", "coordinate", "CHG-404", "--reason", "x"); code != aerr.ExitState {
		t.Errorf("missing change: exit %d", code)
	}
}

func TestCoordinatedCardClipsLongText(t *testing.T) {
	long := strings.Repeat("why ", 600)
	d := coordinatedCard(nil, long)
	if len([]rune(d.Context)) > maxCardContext || len([]rune(d.Recommendation.Reason)) > maxCardContext {
		t.Errorf("card text over the schema's limit: %d %d", len(d.Context), len(d.Recommendation.Reason))
	}
}
