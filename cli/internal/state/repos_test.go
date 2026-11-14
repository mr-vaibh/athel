package state

import (
	"errors"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/build"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// approved is a change whose plan, with one task in repo users, is approved.
func approved(t *testing.T, s *Store) *Change {
	t.Helper()
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Investigating, Specified, AwaitingDecisions, Planned)
	if _, err := s.SetPlan(c.ID, testSession, testPlan(c.ID)); err != nil {
		t.Fatal(err)
	}
	grant(t, s, c.ID, PlanApproved)
	if _, err := s.Transition(c.ID, TransitionRequest{To: PlanApproved, By: ByHuman, Reason: "ok", Session: ptr(testSession)}); err != nil {
		t.Fatal(err)
	}
	return c
}

func work(c *Change) RepoWork {
	return RepoWork{Name: "users", Remote: "github.com/acme/users", Base: "main", BaseHead: strings.Repeat("b", 40),
		Branch: "athel/" + c.ID + "-x", Worktree: "repos/users@" + c.ID}
}

func TestAddRepo(t *testing.T) {
	s, _ := newTestStore(t)
	c := approved(t, s)
	calls := 0
	make := func(c *Change) (RepoWork, error) { calls++; return work(c), nil }
	got, created, err := s.AddRepo(c.ID, testSession, "users", make)
	if err != nil || !created || got != work(c) {
		t.Fatalf("first add: %+v %v %v", got, created, err)
	}
	again, created, err := s.AddRepo(c.ID, testSession, "users", make)
	if err != nil || created || again != got || calls != 1 {
		t.Errorf("second add returns the recorded repo without making it again: %+v %v %v calls=%d", again, created, err, calls)
	}
	if read, _ := s.Get(c.ID); len(read.Repos) != 1 {
		t.Errorf("repos = %+v", read.Repos)
	}
}

func TestAddRepoRefusals(t *testing.T) {
	s, _ := newTestStore(t)
	c := approved(t, s)
	never := func(*Change) (RepoWork, error) { t.Error("make must not run"); return RepoWork{}, nil }
	if _, _, err := s.AddRepo(c.ID, "", "users", never); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, _, err := s.AddRepo(c.ID, testSession, "orders", never); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("a repo the plan does not write in: %v", err)
	}
	if _, _, err := s.AddRepo(c.ID, "other-session", "users", never); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session's lock: %v", err)
	}
	boom := errors.New("git failed")
	if _, _, err := s.AddRepo(c.ID, testSession, "users", func(*Change) (RepoWork, error) { return RepoWork{}, boom }); !errors.Is(err, boom) {
		t.Errorf("make's error is returned: %v", err)
	}
	if read, _ := s.Get(c.ID); len(read.Repos) != 0 {
		t.Errorf("a failed add records nothing: %+v", read.Repos)
	}
	early := mustCreate(t, s, "not approved")
	if _, _, err := s.AddRepo(early.ID, testSession, "users", never); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("before approval: %v", err)
	}
}

func TestUpdateRepo(t *testing.T) {
	s, _ := newTestStore(t)
	c := approved(t, s)
	if _, _, err := s.AddRepo(c.ID, testSession, "users", func(c *Change) (RepoWork, error) { return work(c), nil }); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateRepo(c.ID, testSession, "users", func(r *RepoWork) { r.Pushed = true })
	if err != nil || !got.Repos[0].Pushed {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := s.UpdateRepo(c.ID, testSession, "orders", func(*RepoWork) {}); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("unknown repo: %v", err)
	}
	if _, err := s.UpdateRepo(c.ID, "", "users", func(*RepoWork) {}); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, err := s.UpdateRepo(c.ID, "other", "users", func(*RepoWork) {}); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session: %v", err)
	}
}

func TestWaivers(t *testing.T) {
	s, _ := newTestStore(t)
	c := approved(t, s)
	w := Waiver{Task: "T-01", Subject: "lint", By: ByHuman, Reason: "legacy", Until: "2026-11-30", Grant: "GR-1", At: "2026-11-01T10:00:00.000Z"}
	got, err := s.AddWaiver(c.ID, testSession, w)
	if err != nil || len(got.Waivers) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if !got.Waived("T-01", "lint", "2026-11-30") || got.Waived("T-01", "lint", "2026-12-01") || got.Waived("T-02", "lint", "2026-11-01") {
		t.Error("a waiver holds through its last day, for its task and subject only")
	}
	if _, err := s.AddWaiver(c.ID, "", w); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, err := s.AddWaiver(c.ID, "other", w); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session: %v", err)
	}
}

func TestUpdateBuild(t *testing.T) {
	s, _ := newTestStore(t)
	c := approved(t, s)
	add := func(_ *plan.Plan, recs []build.Task) ([]build.Task, error) {
		return append(recs, build.Task{ID: "T-01", Status: build.InProgress, Started: "x", Commits: []string{}}), nil
	}
	if _, err := s.UpdateBuild(c.ID, testSession, add); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("before building: %v", err)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{To: Building, By: ByAthel, Reason: "go", Session: ptr(testSession)}); err != nil {
		t.Fatal(err)
	}
	got, err := s.UpdateBuild(c.ID, testSession, add)
	if err != nil || len(got.Build) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := s.UpdateBuild(c.ID, "", add); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, err := s.UpdateBuild(c.ID, "other", add); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session: %v", err)
	}
	boom := errors.New("refused")
	if _, err := s.UpdateBuild(c.ID, testSession, func(*plan.Plan, []build.Task) ([]build.Task, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("fn's error: %v", err)
	}
}
