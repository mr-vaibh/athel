package asks

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func newStore(t *testing.T) Store {
	t.Helper()
	return Store{Dir: filepath.Join(t.TempDir(), "asks"), Reg: schema.NewRegistry(), Clock: clock.NewFixed(time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC))}
}

func TestAskLifecycle(t *testing.T) {
	s := newStore(t)
	a, err := s.New(Question, "Who consumes UserUpdated?", "sess-1")
	if err != nil || a.ID != "ASK-001" || a.CreatedAt != "2026-11-09T10:00:00.000Z" || *a.Session != "sess-1" {
		t.Fatalf("new = %+v %v", a, err)
	}
	b, err := s.New(Blast, "Rename phone to phone_number", "")
	if err != nil || b.ID != "ASK-002" || b.Session != nil {
		t.Fatalf("second = %+v %v", b, err)
	}
	got, err := s.Get("ASK-001")
	if err != nil || got.Question != a.Question || got.Kind != Question {
		t.Errorf("get = %+v %v", got, err)
	}
	p, err := s.Write("ASK-001", "investigation.md", []byte("# x\n"))
	if err != nil || filepath.Base(p) != "investigation.md" {
		t.Errorf("write = %s %v", p, err)
	}
}

func TestConcurrentAsks(t *testing.T) {
	s := newStore(t)
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := s.New(Question, "q", "")
			if err != nil {
				t.Error(err)
			}
			ids <- a.ID
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate %s", id)
		}
		seen[id] = true
	}
}

func TestAskErrors(t *testing.T) {
	s := newStore(t)
	if _, err := s.New("other", "q", ""); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad kind: %v", err)
	}
	if _, err := s.Get("ASK-1"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("bad id: %v", err)
	}
	if _, err := s.Get("ASK-009"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing: %v", err)
	}
	if _, err := s.Write("ASK-009", "investigation.md", nil); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("write to a missing ask: %v", err)
	}
	if _, err := s.Write("ASK-001", "../x.md", nil); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("unknown file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "ASK-003"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "ASK-003", "ask.yaml"), []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("ASK-003"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt ask: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "ASK-004", "ask.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("ASK-004"); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable ask: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, ".lock"), 0o700); err == nil {
		if _, err := s.New(Question, "q", ""); !aerr.HasCode(err, aerr.StateIO) {
			t.Errorf("lock is a folder: %v", err)
		}
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	under := Store{Dir: filepath.Join(file, "asks"), Reg: s.Reg, Clock: s.Clock}
	if _, err := under.New(Question, "q", ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("asks under a file: %v", err)
	}
}

func TestAskFolderPermissions(t *testing.T) {
	s := newStore(t)
	if _, err := s.New(Question, "q", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.Dir, 0o700) })
	if _, err := s.New(Question, "q", ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("ask folder that cannot be created: %v", err)
	}
	if err := os.Chmod(s.Dir, 0o300); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(Question, "q", ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("asks folder that cannot be listed: %v", err)
	}
}
