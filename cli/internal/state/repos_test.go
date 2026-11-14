package state

import (
	"errors"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
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
