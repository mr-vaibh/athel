package state

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/changeset"
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
	if got.ChangeSet == nil || got.ChangeSet.Strategy != "single" || got.ChangeSet.Repos[0].Repo != "users" {
		t.Errorf("approval derives the change set: %+v", got.ChangeSet)
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
	if got.State != Planned || got.Approvals.Plan != nil || got.ChangeSet != nil {
		t.Errorf("after a plan change: state %s approval %+v set %+v", got.State, got.Approvals.Plan, got.ChangeSet)
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

func TestMigrateChange1ToCurrent(t *testing.T) {
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
	if !strings.Contains(string(after), "schema: "+SchemaID) || IsLegacy(after) {
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
	legacy := strings.Replace(string(b), "schema: "+SchemaID, "schema: "+SchemaV1, 1)
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

// toLegacy turns a current change file into the change@1 file M0 would have written.
func toLegacy(current string) string {
	return integrityBlock.ReplaceAllString(strings.Replace(withoutBuild(current), "schema: "+SchemaID, "schema: "+SchemaV1, 1), "")
}

// withoutBuild drops the build records version 4 added, which older versions never had.
func withoutBuild(current string) string { return strings.Replace(current, "build: []\n", "", 1) }

// TestApprovalRefusesAnUnorderableSet: two repos that wait on each other cannot be ordered with
// one PR each, so the plan cannot be approved (M5-plan G2).
func TestApprovalRefusesAnUnorderableSet(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned)
	twoWay := "schema: athel/plan@1\nchange: " + c.ID + "\nhash: null\ntasks:\n" +
		"  - {id: T-01, title: a, repo: users, paths: [a.go], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: [build]}\n" +
		"  - {id: T-02, title: b, repo: orders, paths: [b.go], verbs: [edit], covers: [AC-1], depends_on: [T-01], size: S, gates: [build]}\n" +
		"  - {id: T-03, title: c, repo: users, paths: [c.go], verbs: [edit], covers: [AC-1], depends_on: [T-02], size: S, gates: [build]}\n"
	if _, err := s.SetPlan(c.ID, testSession, []byte(twoWay)); err != nil {
		t.Fatal(err)
	}
	grant(t, s, c.ID, PlanApproved)
	_, err := s.Transition(c.ID, TransitionRequest{To: PlanApproved, By: ByHuman, Reason: "ok", Session: ptr(testSession)})
	if !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "wait on each other") {
		t.Fatalf("approval of a two-way plan: %v", err)
	}
	if got, _ := s.Get(c.ID); got.State != Planned || got.ChangeSet != nil {
		t.Errorf("a refused approval changes nothing: %s %+v", got.State, got.ChangeSet)
	}
}

func TestPRsInTheChangeSet(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved)
	if _, err := s.Update(c.ID, func(c *Change) error {
		c.Repos = []RepoWork{{Name: "users", Remote: "github.com/acme/users", Base: "main", BaseHead: strings.Repeat("a", 40),
			Branch: "athel/CHG-001-x", Worktree: "repos/users@CHG-001"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	url := "https://github.com/acme/users/pull/901"
	got, err := s.UpdateRepo(c.ID, testSession, "users", func(w *RepoWork) { w.PR = &url })
	if err != nil {
		t.Fatal(err)
	}
	e := got.ChangeSet.Repos[0]
	if e.PR == nil || *e.PR != "acme/users#901" || e.State == nil || *e.State != "draft" || got.Repos[0].PRRef() != "acme/users#901" {
		t.Errorf("an opened PR is a draft in the set: %+v", e)
	}
	if got, err = s.MarkPR(c.ID, testSession, "users", "ready"); err != nil || *got.ChangeSet.Repos[0].State != "ready" {
		t.Errorf("MarkPR: %+v %v", got.ChangeSet.Repos[0], err)
	}
	if got, _ = s.UpdateRepo(c.ID, testSession, "users", func(*RepoWork) {}); *got.ChangeSet.Repos[0].State != "ready" {
		t.Errorf("a later repo update keeps the PR's state: %+v", got.ChangeSet.Repos[0])
	}
}

func TestMarkPREdges(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.MarkPR(c.ID, "", "users", "ready"); err == nil {
		t.Error("MarkPR needs a session")
	}
	if (RepoWork{}).PRRef() != "" {
		t.Error("no PR, no ref")
	}
	if got, err := s.MarkPR(c.ID, testSession, "users", "ready"); err != nil || got.ChangeSet != nil {
		t.Errorf("a change without a set: %v", err)
	}
}

func TestSetVerify(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	v := &changeset.Verify{At: "2026-11-24T10:00:00.000Z", Result: "pass", Heads: map[string]string{"users@head": strings.Repeat("a", 40)}}
	if _, err := s.SetVerify(c.ID, testSession, v); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("no set yet: %v", err)
	}
	if _, err := s.SetVerify(c.ID, "", v); err == nil {
		t.Error("SetVerify needs a session")
	}
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved)
	got, err := s.SetVerify(c.ID, testSession, v)
	if err != nil || got.ChangeSet.Verify.Result != "pass" {
		t.Errorf("SetVerify: %+v %v", got.ChangeSet, err)
	}
}
