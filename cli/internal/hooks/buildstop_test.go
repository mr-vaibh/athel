package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

const buildPlan = "schema: athel/plan@2\nchange: CHG-001\nhash: null\ntasks:\n" +
	"  - {id: T-01, title: x, repo: users, kind: internal, paths: [src/**], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: []}\n"

// building is a change in building whose build step runs for T-01, held by sess.
func building(t *testing.T, d Deps) *state.Change {
	t.Helper()
	c := newChange(t, d, "x")
	s := d.Store
	move := func(to state.State, by string) {
		t.Helper()
		if _, err := s.Transition(c.ID, state.TransitionRequest{To: to, By: by, Reason: "r", Session: ptrTo(sess)}); err != nil {
			t.Fatalf("to %s: %v", to, err)
		}
	}
	for _, to := range []state.State{state.Investigating, state.Specified, state.AwaitingDecisions, state.Planned} {
		move(to, state.ByAthel)
	}
	if _, err := s.SetPlan(c.ID, sess, []byte(buildPlan)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Grants().Create(c.ID, string(state.PlanApproved), sess, "p1"); err != nil {
		t.Fatal(err)
	}
	move(state.PlanApproved, state.ByHuman)
	move(state.Building, state.ByAthel)
	task := "T-01"
	got, err := s.EnterStep(c.ID, sess, "build", &task)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func ptrTo(s string) *string { return &s }

func stopReq(active bool) *hookio.Request {
	return &hookio.Request{Event: hookio.Stop, SessionID: sess, StopHookActive: active}
}

func TestBuildStopBlocksThenAsks(t *testing.T) {
	d, _ := newDeps(t)
	c := building(t, d)
	if o := Handle(stopReq(true), d); o.Response.BlockStop {
		t.Error("never while already continuing for a stop hook")
	}
	for n := 1; n <= StopBlockLimit; n++ {
		o := Handle(stopReq(false), d)
		if !o.Response.BlockStop || o.Rule != RuleStopBlock || !strings.Contains(o.Response.BlockReason, "Task T-01 is neither done nor blocked") {
			t.Fatalf("stop %d: %+v", n, o)
		}
	}
	o := Handle(stopReq(false), d)
	if o.Response.BlockStop || !strings.Contains(o.Response.SystemMessage, "the step is blocked") {
		t.Fatalf("at the limit: %+v", o)
	}
	got, err := d.Store.Get(c.ID)
	if err != nil || got.State != state.Blocked {
		t.Errorf("the change is blocked: %v %v", got.State, err)
	}
	b, err := os.ReadFile(filepath.Join(d.Store.WS.ChangeDir(c.ID), "decisions.yaml"))
	if err != nil || !strings.Contains(string(b), "Task T-01 stopped 3 times without finishing") {
		t.Errorf("the card: %s %v", b, err)
	}
}

func TestBuildStopNothingToDo(t *testing.T) {
	d, _ := newDeps(t)
	newChange(t, d, "x") // no step running
	if o := Handle(stopReq(false), d); o.Response.BlockStop {
		t.Errorf("no build step: %+v", o)
	}
}

func TestBuildStopErrors(t *testing.T) {
	d, _ := newDeps(t)
	c := building(t, d)
	for range StopBlockLimit {
		Handle(stopReq(false), d)
	}
	dec := filepath.Join(d.Store.WS.ChangeDir(c.ID), "decisions.yaml")
	if err := os.WriteFile(dec, []byte("schema: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o := Handle(stopReq(false), d); o.Result != "error" || !strings.Contains(strings.Join(o.Notes, ";"), "stop card") {
		t.Errorf("an unreadable decisions.yaml: %+v", o)
	}
	other := &hookio.Request{Event: hookio.Stop, SessionID: "sess-B"}
	if o := Handle(other, d); o.Response.BlockStop {
		t.Errorf("another session holds no lock, so no build step of its own: %+v", o)
	}
}

func TestBuildStopTaskDone(t *testing.T) {
	d, _ := newDeps(t)
	c := building(t, d)
	if _, err := d.Store.UpdateBuild(c.ID, sess, func(p *plan.Plan, recs []build.Task) ([]build.Task, error) {
		started, err := build.Start(p, recs, "T-01", "2026-11-01T10:30:00.000Z")
		if err != nil {
			return nil, err
		}
		return build.Finish(started, "T-01", nil, "2026-11-01T11:00:00.000Z")
	}); err != nil {
		t.Fatal(err)
	}
	if o := Handle(stopReq(false), d); o.Response.BlockStop {
		t.Errorf("a done task never blocks: %+v", o)
	}
}
