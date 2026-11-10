package state

import (
	"os"
	"path/filepath"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestSaveChangeFile(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.SaveChangeFile(c.ID, "sess-1", "investigation.md", []byte("# x\n")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "investigation.md")); err != nil || string(b) != "# x\n" {
		t.Errorf("file = %q %v", b, err)
	}
	if _, err := s.SaveChangeFile(c.ID, "sess-2", "investigation.md", nil); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session's write: %v", err)
	}
	if _, err := s.SaveChangeFile(c.ID, "sess-1", "../escape.md", nil); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("unknown file: %v", err)
	}
	if _, err := s.SaveChangeFile(c.ID, "", "investigation.md", nil); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, err := s.Update(c.ID, func(c *Change) error { c.State = Cancelled; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveChangeFile(c.ID, "sess-1", "investigation.md", nil); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("cancelled change: %v", err)
	}
}
