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

func TestIsChangeFile(t *testing.T) {
	for name, want := range map[string]bool{
		"investigation.md": true, "blast-radius.md": true, "prd/v1.index.yaml": true, "prd/v12.index.yaml": true,
		"prd/review-v2.md": true, "prd/v0.index.yaml": false, "prd/v1.md": false, "change.yaml": false,
		"prd/../change.yaml": false, "spec/users.md": false,
	} {
		if IsChangeFile(name) != want {
			t.Errorf("%s: %v, want %v", name, !want, want)
		}
	}
}

func TestSaveChangeFileInSubfolder(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.SaveChangeFile(c.ID, "sess-1", "prd/v1.index.yaml", []byte("x: 1\n")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "prd", "v1.index.yaml")); err != nil || string(b) != "x: 1\n" {
		t.Errorf("file = %q %v", b, err)
	}
}

func TestAddPRDVersion(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if NextPRDVersion(c) != "v2" {
		t.Fatalf("next = %s", NextPRDVersion(c))
	}
	got, err := s.AddPRDVersion(c.ID, "sess-1", "v2", []byte("# v2\n"), "text", []byte("i: 2\n"))
	if err != nil || got.PRD.Current != "v2" || len(got.PRD.Versions) != 2 || got.PRD.Versions[1].Path != "prd/v2.md" {
		t.Fatalf("add: %+v %v", got, err)
	}
	for name, want := range map[string]string{"v2.md": "# v2\n", "v2.index.yaml": "i: 2\n"} {
		if b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "prd", name)); err != nil || string(b) != want {
			t.Errorf("%s = %q %v", name, b, err)
		}
	}
	if _, err := s.AddPRDVersion(c.ID, "sess-1", "v2", nil, "text", nil); !aerr.HasCode(err, aerr.ConflictVersion) {
		t.Errorf("a stale version: %v", err)
	}
	if _, err := s.AddPRDVersion(c.ID, "sess-2", "v3", nil, "text", nil); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("another session: %v", err)
	}
	if _, err := s.AddPRDVersion(c.ID, "", "v3", nil, "text", nil); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: %v", err)
	}
	if _, err := s.Update(c.ID, func(c *Change) error { c.State = Cancelled; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPRDVersion(c.ID, "sess-1", "v3", nil, "text", nil); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("cancelled change: %v", err)
	}
}
