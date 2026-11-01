package state

import (
	"os"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func handEdit(t *testing.T, s *Store, id, old, repl string) {
	t.Helper()
	b, err := os.ReadFile(s.WS.ChangeFile(id))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%q not in change.yaml", old)
	}
	if err := os.WriteFile(s.WS.ChangeFile(id), []byte(strings.Replace(string(b), old, repl, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestHandEditDetected(t *testing.T) {
	tests := []struct {
		name, old, repl string
		tampered        bool
	}{
		{"title", "title: x", "title: y", true},
		{"budget", "max_tool_calls: 2000", "max_tool_calls: 9000", true},
		{"hash removed", "integrity:\n", "integrity: null\nx_was:\n", true},
		{"updated_at only", "updated_at: \"2026-11-01T10:00:00.000Z\"", "updated_at: \"2026-11-02T09:00:00.000Z\"", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestStore(t)
			c := mustCreate(t, s, "x")
			if c.Integrity == nil || !strings.HasPrefix(c.Integrity.Hash, "sha256:") {
				t.Fatalf("not sealed: %+v", c.Integrity)
			}
			old, repl := tc.old, tc.repl
			if tc.name == "hash removed" {
				b, _ := os.ReadFile(s.WS.ChangeFile(c.ID))
				old = string(b)[strings.Index(string(b), "integrity:"):]
				repl = "integrity: null\n"
			}
			handEdit(t, s, c.ID, old, repl)
			got, err := s.Get(c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Tampered() != tc.tampered {
				t.Errorf("tampered = %v", got.Tampered())
			}
		})
	}
}

func TestFrozenUntilAccepted(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	lockFor := func() error { _, err := s.AcquireLock(c.ID, testSession, AcquireOptions{}); return err }
	handEdit(t, s, c.ID, "title: x", "title: edited by hand")
	for name, op := range map[string]func() error{
		"transition": func() error {
			_, err := s.Transition(c.ID, TransitionRequest{To: Investigating, By: ByAthel, Reason: "r", Session: ptr(testSession)})
			return err
		},
		"lock":       lockFor,
		"checkpoint": func() error { _, err := s.AddCheckpoint(c.ID, testSession, "a", nil, "manual"); return err },
	} {
		if err := op(); !aerr.HasCode(err, aerr.IntegrityState) {
			t.Errorf("%s on an edited change: %v", name, err)
		}
	}
	if _, err := s.AcceptIntegrity(c.ID, "", "fixed a typo in the title"); err != nil {
		t.Fatal(err)
	}
	if err := lockFor(); err != nil {
		t.Errorf("after accept: %v", err)
	}
}

func TestAcceptLogged(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	before := c.Integrity.Hash
	if _, err := s.AcceptIntegrity(c.ID, "", "nothing changed"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("accept without an edit: %v", err)
	}
	if _, err := s.AcceptIntegrity(c.ID, "", ""); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("accept without a reason: %v", err)
	}
	handEdit(t, s, c.ID, "title: x", "title: y")
	got, err := s.AcceptIntegrity(c.ID, testSession, "renamed")
	if err != nil || got.Tampered() || got.Title != "y" || got.Integrity.Hash == before {
		t.Fatalf("accept: %+v %v", got, err)
	}
	recs, _ := s.AuditLog().All()
	last := recs[len(recs)-1]
	if last.Kind != audit.KindIntegrityAccept || *last.From != before || *last.To != got.Integrity.Hash || *last.Reason != "renamed" || *last.Session != testSession {
		t.Errorf("accept not audited: %+v", last)
	}
	if again, _ := s.Get(c.ID); again.Tampered() {
		t.Error("still edited after accept")
	}
	if _, err := s.AcceptIntegrity("CHG-404", "", "r"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing change: %v", err)
	}
}

func TestContentHashErrors(t *testing.T) {
	if _, err := ContentHash([]byte("- a list\n")); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("list document: %v", err)
	}
	if _, err := ContentHash([]byte("a: [")); err == nil {
		t.Error("broken YAML hashed")
	}
}
