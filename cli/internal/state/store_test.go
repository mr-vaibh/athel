package state

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestInitIdempotent(t *testing.T) {
	root := t.TempDir()
	_, created, err := Init(root)
	if err != nil || len(created) != 4 {
		t.Fatalf("first Init: created %v, err %v", created, err)
	}
	_, created, err = Init(root)
	if err != nil || len(created) != 0 {
		t.Errorf("second Init: created %v, err %v", created, err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Init(blocker); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("Init under a file: err = %v", err)
	}
}

func TestFindWorkspace(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	ws, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(root); mustEval(t, ws.Root) != want {
		t.Errorf("Find = %s, want %s", ws.Root, root)
	}
	if _, err := Find(t.TempDir()); !aerr.HasCode(err, aerr.StateNotInitialised) {
		t.Errorf("no workspace: err = %v", err)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCreateAllocatesSequentialIDs(t *testing.T) {
	s, _ := newTestStore(t)
	for _, want := range []string{"CHG-001", "CHG-002", "CHG-003"} {
		if c := mustCreate(t, s, "x"); c.ID != want {
			t.Errorf("ID = %s, want %s", c.ID, want)
		}
	}
	prd, err := os.ReadFile(filepath.Join(s.WS.ChangeDir("CHG-001"), "prd", "v1.md"))
	if err != nil || string(prd) != "# PRD\n" {
		t.Errorf("prd/v1.md = %q, %v", prd, err)
	}
	if _, err := os.Stat(filepath.Join(s.WS.ChangeDir("CHG-001"), "artifacts")); err != nil {
		t.Errorf("artifacts dir: %v", err)
	}
}

func TestCreateConcurrentIDsUnique(t *testing.T) {
	s, _ := newTestStore(t)
	const n = 10
	var wg sync.WaitGroup
	idsCh := make(chan string, n)
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := s.Create(NewChange{Title: "x", CreatedBy: "a", PRD: []byte("p"), PRDSource: "text"})
			if err != nil {
				errs <- err
				return
			}
			idsCh <- c.ID
		}()
	}
	wg.Wait()
	close(idsCh)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for id := range idsCh {
		if seen[id] {
			t.Errorf("duplicate ID %s", id)
		}
		seen[id] = true
	}
	if len(seen) != n {
		t.Errorf("got %d IDs, want %d", len(seen), n)
	}
}

func TestCreateErrors(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Create(NewChange{Title: "", CreatedBy: "a", PRD: []byte("p"), PRDSource: "text"}); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("empty title: err = %v", err)
	}
	if _, err := os.Stat(s.WS.ChangeDir("CHG-001")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("half-created change left behind: %v", err)
	}
	if err := os.RemoveAll(s.WS.ChangesDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(NewChange{Title: "x", CreatedBy: "a", PRD: []byte("p"), PRDSource: "text"}); !aerr.HasCode(err, aerr.StateNotInitialised) {
		t.Errorf("no changes dir: err = %v", err)
	}
}

func TestListSortedAndSkipsJunk(t *testing.T) {
	s, _ := newTestStore(t)
	for range 3 {
		mustCreate(t, s, "x")
	}
	if err := os.MkdirAll(filepath.Join(s.WS.ChangesDir(), "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].ID != "CHG-001" || list[2].ID != "CHG-003" {
		t.Errorf("List = %v", list)
	}
	if err := os.WriteFile(s.WS.ChangeFile("CHG-002"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt change: err = %v", err)
	}
	if err := os.RemoveAll(s.WS.ChangesDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("missing dir: err = %v", err)
	}
}

func TestUpdate(t *testing.T) {
	s, clk := newTestStore(t)
	c := mustCreate(t, s, "x")
	clk.Advance(90 * 1e9)
	got, err := s.Update(c.ID, func(c *Change) error {
		c.Title = "renamed"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "renamed" || got.UpdatedAt != "2026-11-01T10:01:30.000Z" {
		t.Errorf("updated = %+v", got)
	}
	sentinel := errors.New("stop")
	if _, err := s.Update(c.ID, func(c *Change) error { c.Title = "lost"; return sentinel }); !errors.Is(err, sentinel) {
		t.Errorf("fn error: %v", err)
	}
	reloaded, _ := s.Get(c.ID)
	if reloaded.Title != "renamed" {
		t.Errorf("failed update was written: %q", reloaded.Title)
	}
	if _, err := s.Update(c.ID, func(c *Change) error { c.State = "bogus"; return nil }); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid result: err = %v", err)
	}
	for _, id := range []string{"CHG-404", "bad"} {
		if _, err := s.Update(id, func(*Change) error { return nil }); err == nil {
			t.Errorf("Update(%s) succeeded", id)
		}
	}
	if err := os.WriteFile(s.WS.ChangeFile(c.ID), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(c.ID, func(*Change) error { return nil }); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt file: err = %v", err)
	}
}
