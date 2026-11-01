package state

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestEveryStepHasStates(t *testing.T) {
	want := []string{"blast-radius", "build", "decide", "git-preflight", "investigate", "learn", "plan",
		"review", "ship", "spec", "start", "uat", "verify"}
	if got := strings.Join(Steps(), ","); got != strings.Join(want, ",") {
		t.Errorf("Steps() = %s", got)
	}
	for _, step := range want {
		for _, st := range stepStates[step] {
			if !IsWorking(st) {
				t.Errorf("step %s may run in non-working state %s", step, st)
			}
		}
	}
}

func TestEnterRecordsAttempt(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating)
	for want := 1; want <= 2; want++ {
		got, err := s.EnterStep(c.ID, testSession, "investigate", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Step.Attempt != want || got.StepAttempts["investigate"] != want || got.Step.Status != StepRunning || got.Lock.Session != testSession {
			t.Errorf("attempt %d: step %+v attempts %v", want, got.Step, got.StepAttempts)
		}
	}
	task := "T-07"
	walkFrom := mustCreate(t, s, "y")
	walk(t, s, walkFrom.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved, Building)
	got, err := s.EnterStep(walkFrom.ID, testSession, "build", &task)
	if err != nil || *got.Step.Task != "T-07" {
		t.Errorf("build with task: %+v %v", got, err)
	}
	events := readStateEvents(t, s)
	if last := events[len(events)-1]; last.Kind != EventStepEnter || *last.Step != "build" {
		t.Errorf("last event = %+v", last)
	}
}

func TestEnterIllegalStepForState(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	bad := "T7"
	tests := []struct {
		name    string
		session string
		step    string
		task    *string
		code    aerr.Code
	}{
		{"wrong state", testSession, "build", nil, aerr.StateIllegalTransition},
		{"unknown step", testSession, "deploy", nil, aerr.StateInvalid},
		{"bad task", testSession, "start", &bad, aerr.StateInvalid},
		{"no session", "", "start", nil, aerr.StateNoSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.EnterStep(c.ID, tt.session, tt.step, tt.task); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	if _, err := s.EnterStep(c.ID, testSession, "start", nil); err != nil {
		t.Fatal(err)
	}
	walk(t, s, c.ID, Investigating)
	if _, err := s.EnterStep(c.ID, testSession, "investigate", nil); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("entering while another step runs: err = %v", err)
	}
	if _, err := s.EnterStep(c.ID, "other", "start", nil); err == nil {
		t.Error("another session entered a step")
	}
}

func TestExitResults(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.EnterStep(c.ID, testSession, "start", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.ExitStep(c.ID, testSession, "start", StepOK, "")
	if err != nil || got.Step.Status != StepOK || got.Step.EndedAt == nil || got.Step.Reason != nil {
		t.Fatalf("exit ok: %+v %v", got.Step, err)
	}
	walk(t, s, c.ID, Investigating)
	if _, err := s.EnterStep(c.ID, testSession, "investigate", nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ExitStep(c.ID, testSession, "investigate", StepFailed, "grafana unreachable")
	if err != nil || got.Step.Status != StepFailed || *got.Step.Reason != "grafana unreachable" || got.State != Investigating {
		t.Fatalf("exit failed: %+v %s %v", got.Step, got.State, err)
	}
	events := readStateEvents(t, s)
	if last := events[len(events)-1]; last.Kind != EventStepExit || *last.Reason != StepFailed {
		t.Errorf("last event = %+v", last)
	}
}

func TestExitBlockedMovesToBlocked(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating)
	if _, err := s.EnterStep(c.ID, testSession, "investigate", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.ExitStep(c.ID, testSession, "investigate", StepBlocked, "kubectl auth expired")
	if err != nil {
		t.Fatal(err)
	}
	last := got.History[len(got.History)-1]
	if got.State != Blocked || *got.BlockedFrom != Investigating || last.To != Blocked || last.By != ByAthel ||
		!strings.Contains(last.Reason, "kubectl auth expired") {
		t.Errorf("state %s from %v, history %+v", got.State, got.BlockedFrom, last)
	}
}

func TestExitErrors(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	tests := []struct {
		name, session, step, result, reason string
		code                                aerr.Code
	}{
		{"no session", "", "start", StepOK, "", aerr.StateNoSession},
		{"bad result", testSession, "start", "done", "", aerr.StateInvalid},
		{"blocked without reason", testSession, "start", StepBlocked, "", aerr.StateInvalid},
		{"failed without reason", testSession, "start", StepFailed, " ", aerr.StateInvalid},
		{"not running", testSession, "start", StepOK, "", aerr.StateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.ExitStep(c.ID, tt.session, tt.step, tt.result, tt.reason); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	if _, err := s.EnterStep(c.ID, testSession, "start", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExitStep(c.ID, "intruder", "start", StepOK, ""); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("exit by another session: err = %v", err)
	}
}

func TestCheckpointAppendAndLast(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if LastCheckpoint(c) != nil {
		t.Error("new change has a checkpoint")
	}
	if _, err := s.AddCheckpoint(c.ID, testSession, "intake.prd-copied", nil, TriggerManual); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnterStep(c.ID, testSession, "start", nil); err != nil {
		t.Fatal(err)
	}
	note := "PRD has no acceptance criteria"
	got, err := s.AddCheckpoint(c.ID, testSession, "start.gaps-listed", &note, TriggerPreCompact)
	if err != nil {
		t.Fatal(err)
	}
	last := LastCheckpoint(got)
	if len(got.Checkpoints) != 2 || last.Name != "start.gaps-listed" || *last.Step != "start" || *last.Note != note ||
		last.Trigger != TriggerPreCompact || got.Checkpoints[0].Step != nil {
		t.Errorf("checkpoints = %+v", got.Checkpoints)
	}
	for _, bad := range []string{"", "Upper", "-lead", strings.Repeat("a", 129), "has space"} {
		if _, err := s.AddCheckpoint(c.ID, testSession, bad, nil, TriggerManual); !aerr.HasCode(err, aerr.StateInvalid) {
			t.Errorf("name %q: err = %v", bad, err)
		}
	}
	if _, err := s.AddCheckpoint(c.ID, "", "a", nil, TriggerManual); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: err = %v", err)
	}
	if _, err := s.AddCheckpoint(c.ID, "other", "a", nil, TriggerManual); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("other session: err = %v", err)
	}
}

func TestEnterResetsStopBlocks(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.Update(c.ID, func(c *Change) error { c.StopBlocks.Count = 2; return nil }); err != nil {
		t.Fatal(err)
	}
	got, err := s.EnterStep(c.ID, testSession, "start", nil)
	if err != nil || got.StopBlocks.Count != 0 {
		t.Errorf("stop blocks = %d, %v", got.StopBlocks.Count, err)
	}
}
