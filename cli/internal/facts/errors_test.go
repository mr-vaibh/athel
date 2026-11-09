package facts

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestDraftWithoutDerivedFrom(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t, func(d *Draft) { d.DerivedFrom = nil }))
	if f.DerivedFrom == nil || len(f.DerivedFrom) != 0 {
		t.Errorf("derived_from = %#v, want an empty list", f.DerivedFrom)
	}
}

func TestEncodeErrors(t *testing.T) {
	s, _ := newStore(t)
	nan := draft(t, func(d *Draft) { d.Sources[0].Sampling = math.NaN() })
	if _, _, err := s.Add(nan); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("NaN sampling: %v, want E-STATE-SCHEMA", err)
	}
	f := mustAdd(t, s, draft(t))
	if _, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "bad session id"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid session: %v, want E-STATE-SCHEMA", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "access.jsonl")); err == nil {
		t.Error("an invalid access record was written")
	}
}

func TestLockErrors(t *testing.T) {
	s, _ := newStore(t)
	s.LockTimeout = 0 // the default applies
	mustAdd(t, s, draft(t))
	if err := os.Remove(filepath.Join(s.Dir, ".lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.Dir, ".lock"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(Query{}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("lock file is a directory: %v, want E-STATE-IO", err)
	}
}

func TestGetErrors(t *testing.T) {
	s, _ := newStore(t)
	if _, _, err := s.Get("F-1"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("bad id: %v", err)
	}
	if _, _, err := s.Get("F-0001"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("unknown fact: %v", err)
	}
	mustAdd(t, s, draft(t))
	if err := os.MkdirAll(s.archivePath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get("F-0001"); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable archive: %v", err)
	}
	if _, _, err := s.Add(draft(t)); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("add with an unreadable archive: %v", err)
	}
}

func TestUseReadErrors(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	f.VerifiedAt = "yesterday"
	appendRaw(t, s, f)
	if _, err := s.Use(f.ID, Orientation, Ref{Ask: "ASK-001"}, ""); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad verified_at: %v", err)
	}
	if _, err := s.Archive(false); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("archive with a bad verified_at: %v", err)
	}
	empty, _ := newStore(t)
	if err := os.MkdirAll(empty.factsPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Use("F-0001", Orientation, Ref{Ask: "ASK-001"}, ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable cache: %v", err)
	}
	if _, err := empty.Archive(false); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("archive of an unreadable cache: %v", err)
	}
}

func TestOverlongLineFailsClosed(t *testing.T) {
	s, _ := newStore(t)
	mustAdd(t, s, draft(t))
	appendLine(t, s, strings.Repeat("x", maxLine+1))
	if _, err := s.Find(Query{}); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("err = %v, want E-STATE-SCHEMA", err)
	}
}

func TestArchiveAccessLogErrors(t *testing.T) {
	tests := map[string]string{
		"not json":     "{\n",
		"invalid time": `{"schema":"athel/fact-access@1","at":"soon","fact":"F-0001"}` + "\n",
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			s, _ := newStore(t)
			mustAdd(t, s, draft(t))
			if err := os.WriteFile(s.accessPath(), []byte(line), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Archive(true); !aerr.HasCode(err, aerr.StateSchema) {
				t.Errorf("err = %v, want E-STATE-SCHEMA", err)
			}
		})
	}
	s, _ := newStore(t)
	mustAdd(t, s, draft(t))
	if err := os.MkdirAll(s.accessPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(true); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable access log: %v", err)
	}
}

// archivable returns a store with one fact old enough to be archived.
func archivable(t *testing.T) *Store {
	t.Helper()
	s, clk := newStore(t)
	mustAdd(t, s, draft(t))
	mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "staging" }))
	clk.Advance(91 * 24 * time.Hour)
	return s
}

func TestArchiveWriteErrors(t *testing.T) {
	s := archivable(t)
	if err := os.WriteFile(filepath.Join(s.Dir, "archive"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(false); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("archive folder is a file: %v", err)
	}
	s = archivable(t)
	if err := os.MkdirAll(s.archivePath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Archive(false); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("archive file is a folder: %v", err)
	}
	s = archivable(t)
	if err := os.MkdirAll(filepath.Dir(s.archivePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.Dir, 0o700) })
	if _, err := s.Archive(false); err == nil {
		t.Error("rewriting the active file in a read-only folder succeeded")
	}
}
