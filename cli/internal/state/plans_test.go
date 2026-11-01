package state

import (
	"os"
	"regexp"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestApprovalBindsToPlanHash(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned)
	grant(t, s, c.ID, PlanApproved) // a failed attempt must not use it up
	if _, err := s.Transition(c.ID, TransitionRequest{To: PlanApproved, By: ByHuman, Reason: "ok", Session: ptr(testSession)}); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Fatalf("approval without plan: %v", err)
	}
	if _, err := s.SetPlan(c.ID, testSession, testPlan(c.ID)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Transition(c.ID, TransitionRequest{To: PlanApproved, By: ByHuman, Reason: "ok", Session: ptr(testSession), PromptID: ptr("p-9")})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := s.ApprovedPlan(got)
	if !ok || got.Approvals.Plan.Hash != *p.Hash || *got.Approvals.Plan.PromptID != "p-9" || got.Approvals.Plan.By != ByHuman {
		t.Errorf("approval = %+v", got.Approvals.Plan)
	}
	last := got.History[len(got.History)-1]
	if len(last.Evidence) != 1 || last.Evidence[0].Path != "plan.yaml" || last.Evidence[0].SHA256 == "" {
		t.Errorf("approval evidence = %+v", last.Evidence)
	}
}

func TestApprovalVoidOnPlanHashChange(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved, Building)
	same, err := s.SetPlan(c.ID, testSession, testPlan(c.ID))
	if err != nil || same.State != Building || same.Approvals.Plan == nil {
		t.Fatalf("setting the identical plan must keep the approval: %v %+v", err, same)
	}
	changed := []byte(strings.Replace(string(testPlan(c.ID)), "src/x.go", "src/y.go", 1))
	got, err := s.SetPlan(c.ID, testSession, changed)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Planned || got.Approvals.Plan != nil {
		t.Errorf("after a plan change: state %s approval %+v", got.State, got.Approvals.Plan)
	}
	if _, ok := s.ApprovedPlan(got); ok {
		t.Error("ApprovedPlan true after the plan changed")
	}
	recs, _ := s.AuditLog().All()
	if last := recs[len(recs)-1]; last.Kind != EventTransition || !strings.Contains(*last.Reason, "approval void") {
		t.Errorf("void not audited: %+v", last)
	}
}

func TestHandEditedPlanIsNotApproved(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved)
	got, _ := s.Get(c.ID)
	if _, ok := s.ApprovedPlan(got); !ok {
		t.Fatal("approved plan not recognised")
	}
	b, _ := os.ReadFile(s.WS.PlanFile(c.ID))
	if err := os.WriteFile(s.WS.PlanFile(c.ID), []byte(strings.Replace(string(b), "src/x.go", "src/anything.go", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ApprovedPlan(got); ok {
		t.Error("a hand-edited plan must not count as approved")
	}
	other := mustCreate(t, s, "y")
	walk(t, s, other.ID, Investigating, Specified, AwaitingDecisions, Planned)
	if _, err := s.SetPlan(other.ID, testSession, testPlan(other.ID)); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(s.WS.PlanFile(other.ID))
	if err := os.WriteFile(s.WS.PlanFile(other.ID), []byte(strings.Replace(string(b), "src/x.go", "src/anything.go", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(other.ID, TransitionRequest{To: PlanApproved, By: ByHuman, Reason: "ok", Session: ptr(testSession)}); !aerr.HasCode(err, aerr.IntegrityState) {
		t.Errorf("approving a hand-edited plan: %v", err)
	}
}

func TestSetPlanErrors(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	tests := []struct {
		name    string
		session string
		data    []byte
		code    aerr.Code
	}{
		{"no session", "", testPlan(c.ID), aerr.StateNoSession},
		{"invalid plan", testSession, []byte("schema: athel/plan@1\n"), aerr.StateSchema},
		{"other change", testSession, testPlan("CHG-099"), aerr.StateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.SetPlan(c.ID, tt.session, tt.data); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	walk(t, s, c.ID, Cancelled)
	if _, err := s.SetPlan(c.ID, testSession, testPlan(c.ID)); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("plan of a cancelled change: %v", err)
	}
	if _, err := s.LoadPlan("CHG-404"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing plan: %v", err)
	}
}

func TestMigrateChange1To2(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	mustCreate(t, s, "already v2")
	b, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	legacy := toLegacy(string(b))
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := s.Get(c.ID)
	if err != nil || read.Schema != SchemaID {
		t.Fatalf("legacy file read: %+v %v", read, err)
	}
	ids, err := s.Migrate(true)
	if err != nil || strings.Join(ids, ",") != c.ID {
		t.Fatalf("dry run = %v %v", ids, err)
	}
	if after, _ := os.ReadFile(s.WS.ChangeFile(c.ID)); !strings.Contains(string(after), "change@1") {
		t.Error("dry run wrote the file")
	}
	if ids, err := s.Migrate(false); err != nil || len(ids) != 1 {
		t.Fatalf("migrate = %v %v", ids, err)
	}
	after, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	if !strings.Contains(string(after), "schema: athel/change@2") || IsLegacy(after) {
		t.Errorf("not upgraded:\n%s", after)
	}
	if ids, _ := s.Migrate(false); len(ids) != 0 {
		t.Errorf("second migrate = %v", ids)
	}
}

func TestMigrateErrors(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	b, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
	legacy := strings.Replace(string(b), "schema: athel/change@2", "schema: athel/change@1", 1)
	badLegacy := strings.Replace(legacy, "state: draft", "state: bogus", 1)
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte(badLegacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Migrate(false); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid legacy file: %v", err)
	}
	if err := os.RemoveAll(s.WS.ChangesDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Migrate(false); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("no changes dir: %v", err)
	}
}

var integrityBlock = regexp.MustCompile(`(?m)^integrity:\n(  .*\n)*`)

// toLegacy turns a change@2 file into the change@1 file M0 would have written.
func toLegacy(v2 string) string {
	return integrityBlock.ReplaceAllString(strings.Replace(v2, "schema: athel/change@2", "schema: athel/change@1", 1), "")
}
