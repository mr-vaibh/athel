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
		"prd/../change.yaml": false, "spec/users.md": true, "spec/../x.md": false, "spec/a/b.md": false,
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

func TestUpdateChangeFile(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	appendLine := func(line string) func([]byte) ([]byte, error) {
		return func(old []byte) ([]byte, error) { return append(old, []byte(line+"\n")...), nil }
	}
	for _, line := range []string{"a", "b"} {
		if _, err := s.UpdateChangeFile(c.ID, "sess-1", "decisions.yaml", appendLine(line)); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "decisions.yaml")); err != nil || string(b) != "a\nb\n" {
		t.Errorf("file = %q %v", b, err)
	}
	failing := func([]byte) ([]byte, error) { return nil, aerr.New(aerr.StateInvalid, "nope") }
	if _, err := s.UpdateChangeFile(c.ID, "sess-1", "decisions.yaml", failing); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("fn error: %v", err)
	}
	for name, tt := range map[string]struct {
		session, file string
		code          aerr.Code
	}{
		"no session":    {"", "decisions.yaml", aerr.StateNoSession},
		"unknown file":  {"sess-1", "change.yaml", aerr.StateInvalid},
		"other session": {"sess-2", "decisions.yaml", aerr.StateLockHeld},
	} {
		if _, err := s.UpdateChangeFile(c.ID, tt.session, tt.file, appendLine("x")); !aerr.HasCode(err, tt.code) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(s.WS.ChangeDir(c.ID), "investigation.md"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateChangeFile(c.ID, "sess-1", "investigation.md", appendLine("x")); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable file: %v", err)
	}
	if _, err := s.Update(c.ID, func(c *Change) error { c.State = Cancelled; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateChangeFile(c.ID, "sess-1", "decisions.yaml", appendLine("x")); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("cancelled change: %v", err)
	}
}

func TestSaveChangeFileCreatesFolder(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.SaveChangeFile(c.ID, "sess-1", "spec/users.md", []byte("# users\n")); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(c.ID), "spec", "users.md")); err != nil || string(b) != "# users\n" {
		t.Errorf("file = %q %v", b, err)
	}
	d := mustCreate(t, s, "y")
	if err := os.WriteFile(filepath.Join(s.WS.ChangeDir(d.ID), "spec"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveChangeFile(d.ID, "sess-1", "spec/users.md", nil); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("a file where the folder belongs: %v", err)
	}
	write := func([]byte) ([]byte, error) { return []byte("x"), nil }
	if _, err := s.UpdateChangeFile(d.ID, "sess-1", "spec/users.md", write); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("update with a file where the folder belongs: %v", err)
	}
}
