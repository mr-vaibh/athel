package state

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// specEdges is docs/plans/M0-plan.md §6.3 written out independently of the code, edge by edge.
var specEdges = map[[2]State]bool{
	{Draft, Investigating}: true, {Investigating, Specified}: true, {Specified, AwaitingDecisions}: true,
	{AwaitingDecisions, Planned}: true, {Planned, PlanApproved}: true, {PlanApproved, Building}: true,
	{Building, ReadyForPR}: true, {ReadyForPR, InReview}: true, {InReview, Verifying}: true,
	{Verifying, UAT}: true, {UAT, Done}: true,
	{UAT, Specified}: true, {Verifying, Building}: true,
	{PlanApproved, Planned}: true, {Building, Planned}: true, {ReadyForPR, Planned}: true,
}

func specAllowed(from, to State, blockedFrom State) bool {
	working := map[State]bool{Draft: true, Investigating: true, Specified: true, AwaitingDecisions: true,
		Planned: true, PlanApproved: true, Building: true, ReadyForPR: true, InReview: true, Verifying: true, UAT: true}
	switch {
	case to == Cancelled:
		return from != Done && from != Cancelled
	case to == Blocked:
		return working[from]
	case from == Blocked:
		return to == blockedFrom
	}
	return specEdges[[2]State{from, to}]
}

const testSession = "sess-1"

func TestTransitionTableExhaustive(t *testing.T) {
	if len(AllStates) != 14 {
		t.Fatalf("AllStates has %d states, want 14", len(AllStates))
	}
	origin := Building
	for _, from := range AllStates {
		for _, to := range AllStates {
			var bf *State
			if from == Blocked {
				bf = &origin
			}
			if got, want := Allowed(from, to, bf), specAllowed(from, to, origin); got != want {
				t.Errorf("Allowed(%s → %s) = %v, want %v", from, to, got, want)
			}
		}
	}
	if Allowed(Blocked, Draft, nil) {
		t.Error("blocked without a recorded origin must not return anywhere")
	}
}

func TestHumanOnlyTargets(t *testing.T) {
	for _, s := range AllStates {
		if want := s == PlanApproved || s == Done; RequiresHuman(s) != want {
			t.Errorf("RequiresHuman(%s) = %v, want %v", s, RequiresHuman(s), want)
		}
	}
}

// walk moves c through the given states with by=human, failing the test on any error.
func walk(t *testing.T, s *Store, id string, states ...State) {
	t.Helper()
	for _, to := range states {
		if _, err := s.Transition(id, TransitionRequest{Session: ptr(testSession), To: to, By: ByHuman, Reason: "move to " + string(to)}); err != nil {
			t.Fatalf("→ %s: %v", to, err)
		}
	}
}

func TestTransitionHappyPathRecordsHistory(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	session, prompt := testSession, "prompt-1"
	got, err := s.Transition(c.ID, TransitionRequest{
		To: Investigating, By: ByAthel, Reason: "start", Evidence: []string{"prd/v1.md"},
		Session: &session, PromptID: &prompt,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := got.History[0]
	if got.State != Investigating || h.Seq != 1 || h.From != Draft || h.To != Investigating || h.By != ByAthel {
		t.Errorf("history = %+v", h)
	}
	if len(h.Evidence) != 1 || h.Evidence[0].Path != "prd/v1.md" || h.Evidence[0].SHA256 != got.PRD.Versions[0].SHA256 {
		t.Errorf("evidence = %+v", h.Evidence)
	}
	if *h.Origin.Session != session || *h.Origin.PromptID != prompt {
		t.Errorf("origin = %+v", h.Origin)
	}
}

func TestTransitionLoopsKeepSequentialHistory(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved, Building, ReadyForPR,
		InReview, Verifying, Building, ReadyForPR, InReview, Verifying, UAT, Specified)
	final, _ := s.Get(c.ID)
	if final.State != Specified || len(final.History) != 15 {
		t.Errorf("state %s, %d history entries", final.State, len(final.History))
	}
	for i, h := range final.History {
		if h.Seq != i+1 {
			t.Errorf("history[%d].seq = %d", i, h.Seq)
		}
	}
}

func TestHistoryAppendOnly(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating)
	before, _ := s.Get(c.ID)
	walk(t, s, c.ID, Specified)
	after, _ := s.Get(c.ID)
	if len(after.History) != len(before.History)+1 || !reflect.DeepEqual(after.History[0], before.History[0]) {
		t.Errorf("earlier history changed: %+v vs %+v", before.History, after.History)
	}
}

func TestBlockedReturnsOnlyToOrigin(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Blocked)
	got, _ := s.Get(c.ID)
	if got.State != Blocked || got.BlockedFrom == nil || *got.BlockedFrom != Investigating {
		t.Fatalf("blocked state = %s from %v", got.State, got.BlockedFrom)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{Session: ptr(testSession), To: Specified, By: ByHuman, Reason: "skip"}); !aerr.HasCode(err, aerr.StateIllegalTransition) {
		t.Errorf("blocked → other state: err = %v", err)
	}
	walk(t, s, c.ID, Investigating)
	got, _ = s.Get(c.ID)
	if got.State != Investigating || got.BlockedFrom != nil {
		t.Errorf("after unblock: %s from %v", got.State, got.BlockedFrom)
	}
	other := mustCreate(t, s, "draft can block")
	walk(t, s, other.ID, Blocked, Draft)
}

func TestCancelledFromAnyNonTerminal(t *testing.T) {
	s, _ := newTestStore(t)
	blocked := mustCreate(t, s, "x")
	walk(t, s, blocked.ID, Blocked, Cancelled)
	if _, err := s.Transition(blocked.ID, TransitionRequest{Session: ptr(testSession), To: Cancelled, By: ByHuman, Reason: "again"}); !aerr.HasCode(err, aerr.StateIllegalTransition) {
		t.Errorf("cancelled → cancelled: err = %v", err)
	}
	if _, err := s.Transition(blocked.ID, TransitionRequest{Session: ptr(testSession), To: Draft, By: ByHuman, Reason: "revive"}); !aerr.HasCode(err, aerr.StateIllegalTransition) {
		t.Errorf("cancelled is not terminal: err = %v", err)
	}
	done := mustCreate(t, s, "y")
	walk(t, s, done.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved, Building,
		ReadyForPR, InReview, Verifying, UAT, Done)
	if _, err := s.Transition(done.ID, TransitionRequest{Session: ptr(testSession), To: Cancelled, By: ByHuman, Reason: "late"}); !aerr.HasCode(err, aerr.StateIllegalTransition) {
		t.Errorf("done → cancelled: err = %v", err)
	}
}

func TestHumanRequiredForApprovalAndDone(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned)
	_, err := s.Transition(c.ID, TransitionRequest{Session: ptr(testSession), To: PlanApproved, By: ByAthel, Reason: "approve myself"})
	if !aerr.HasCode(err, aerr.PolicyHumanRequired) || aerr.ExitCode(err) != aerr.ExitPolicy {
		t.Fatalf("err = %v (exit %d), want E-POLICY-HUMAN-REQUIRED", err, aerr.ExitCode(err))
	}
	got, _ := s.Get(c.ID)
	if got.State != Planned {
		t.Errorf("state changed to %s", got.State)
	}
	events := readStateEvents(t, s)
	last := events[len(events)-1]
	if last.Kind != EventHumanRequired || last.ErrorCode == nil || *last.ErrorCode != string(aerr.PolicyHumanRequired) {
		t.Errorf("last event = %+v", last)
	}
}

func TestIllegalTransitionAudited(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	_, err := s.Transition(c.ID, TransitionRequest{Session: ptr(testSession), To: Building, By: ByHuman, Reason: "skip ahead"})
	if !aerr.HasCode(err, aerr.StateIllegalTransition) || aerr.ExitCode(err) != aerr.ExitState {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "allowed: investigating, blocked, cancelled") {
		t.Errorf("error does not list allowed states: %v", err)
	}
	events := readStateEvents(t, s)
	if len(events) != 1 || events[0].Kind != EventIllegalTransition || *events[0].From != "draft" || *events[0].To != "building" {
		t.Fatalf("events = %+v", events)
	}
	reg := schema.NewRegistry()
	for _, line := range readStateEventLines(t, s) {
		var doc any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatal(err)
		}
		if err := reg.Validate(EventSchemaID, doc); err != nil {
			t.Errorf("event does not match schema: %v", err)
		}
	}
}

func TestTransitionRequestValidation(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	tests := []struct {
		name string
		req  TransitionRequest
		code aerr.Code
	}{
		{"bad actor", TransitionRequest{Session: ptr(testSession), To: Investigating, By: "bot", Reason: "r"}, aerr.StateInvalid},
		{"empty reason", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "  "}, aerr.StateInvalid},
		{"unknown state", TransitionRequest{Session: ptr(testSession), To: "shipped", By: ByHuman, Reason: "r"}, aerr.StateInvalid},
		{"self transition", TransitionRequest{Session: ptr(testSession), To: Draft, By: ByHuman, Reason: "r"}, aerr.StateIllegalTransition},
		{"absolute evidence", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "r", Evidence: []string{"/etc/hosts"}}, aerr.StateInvalid},
		{"escaping evidence", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "r", Evidence: []string{"../CHG-002/change.yaml"}}, aerr.StateInvalid},
		{"dot evidence", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "r", Evidence: []string{"."}}, aerr.StateInvalid},
		{"missing evidence", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "r", Evidence: []string{"prd/v9.md"}}, aerr.StateIO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Transition(c.ID, tt.req); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	if got, _ := s.Get(c.ID); got.State != Draft || len(got.History) != 0 {
		t.Errorf("rejected requests changed the change: %s, %d history", got.State, len(got.History))
	}
	if _, err := s.Transition("CHG-404", TransitionRequest{Session: ptr(testSession), To: Investigating, By: ByHuman, Reason: "r"}); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing change: err = %v", err)
	}
}

func TestTransitionFailsWhenAuditUnwritable(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if err := os.RemoveAll(s.WS.AuditDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.WS.AuditDir(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.Transition(c.ID, TransitionRequest{Session: ptr(testSession), To: Building, By: ByHuman, Reason: "r"})
	if !aerr.HasCode(err, aerr.StateIllegalTransition) || !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("err = %v, want both the transition and the audit failure", err)
	}
}

func TestNextStates(t *testing.T) {
	tests := []struct {
		state State
		bf    *State
		want  string
	}{
		{Draft, nil, "investigating, blocked, cancelled"},
		{PlanApproved, nil, "planned, building, blocked, cancelled"},
		{Blocked, ptr(Verifying), "verifying, cancelled"},
		{Done, nil, "none (terminal)"},
	}
	for _, tt := range tests {
		if got := joinStates(NextStates(&Change{State: tt.state, BlockedFrom: tt.bf})); got != tt.want {
			t.Errorf("NextStates(%s) = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func readStateEventLines(t *testing.T, s *Store) []string {
	t.Helper()
	f, err := os.Open(filepath.Clean(s.WS.StateEventsFile()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func readStateEvents(t *testing.T, s *Store) []Event {
	t.Helper()
	var out []Event
	for _, line := range readStateEventLines(t, s) {
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}
