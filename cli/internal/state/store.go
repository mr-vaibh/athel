package state

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/ids"
	"github.com/mr-vaibh/athel/cli/internal/lock"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// DefaultLockTimeout bounds how long a CLI call waits for another athel process.
const DefaultLockTimeout = 5 * time.Second

// Store reads and writes changes in one workspace.
type Store struct {
	WS          Workspace
	Reg         *schema.Registry
	Clock       clock.Clock
	LockTimeout time.Duration
	// TTLMin and HeartbeatEvery tune the session lock; zero means the defaults.
	TTLMin         int
	HeartbeatEvery time.Duration
}

// NewStore returns a store with the default lock timeout.
func NewStore(ws Workspace, reg *schema.Registry, clk clock.Clock) *Store {
	return &Store{WS: ws, Reg: reg, Clock: clk, LockTimeout: DefaultLockTimeout}
}

func (s *Store) now() string { return clock.Format(s.Clock.Now()) }

// Get loads a change by ID.
func (s *Store) Get(id string) (*Change, error) {
	cid, err := ids.ParseChange(id)
	if err != nil {
		return nil, err
	}
	return Load(s.Reg, s.WS.ChangeFile(cid.String()))
}

// List loads every change, ordered by ID. Folders that are not change IDs are ignored.
func (s *Store) List() ([]*Change, error) {
	entries, err := os.ReadDir(s.WS.ChangesDir())
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "list changes", err)
	}
	type item struct {
		id ids.ChangeID
		c  *Change
	}
	var items []item
	for _, e := range entries {
		cid, err := ids.ParseChange(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		c, err := Load(s.Reg, s.WS.ChangeFile(cid.String()))
		if err != nil {
			return nil, err
		}
		items = append(items, item{cid, c})
	}
	slices.SortFunc(items, func(a, b item) int {
		return cmp.Or(cmp.Compare(a.id.Number, b.id.Number), cmp.Compare(a.id.Child, b.id.Child))
	})
	out := make([]*Change, len(items))
	for i, it := range items {
		out[i] = it.c
	}
	return out, nil
}

// Update runs fn on the change under its OS lock and saves the result with a fresh
// updated_at. If fn returns an error nothing is written.
func (s *Store) Update(id string, fn func(*Change) error) (*Change, error) {
	cid, err := ids.ParseChange(id)
	if err != nil {
		return nil, err
	}
	id = cid.String()
	if _, err := os.Stat(s.WS.ChangeDir(id)); err != nil {
		return nil, aerr.Wrap(aerr.StateNotFound, "no change "+id, err)
	}
	l, err := lock.Acquire(s.WS.ChangeLockFile(id), s.LockTimeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = l.Release() }() // release errors cannot affect the already-saved state
	c, err := Load(s.Reg, s.WS.ChangeFile(id))
	if err != nil {
		return nil, err
	}
	if err := fn(c); err != nil {
		return nil, err
	}
	c.UpdatedAt = s.now()
	if err := Save(s.Reg, s.WS.ChangeFile(id), c); err != nil {
		return nil, err
	}
	return c, nil
}

// NewChange describes a change to create.
type NewChange struct {
	Title     string
	CreatedBy string
	PRD       []byte
	PRDSource string // file | text | stdin
	Budget    Budget // limits only; usage starts at zero
}

// Create allocates the next change ID under the workspace lock and writes prd/v1.md and
// change.yaml. The change starts in draft.
func (s *Store) Create(n NewChange) (*Change, error) {
	l, err := lock.Acquire(s.WS.LockFile(), s.LockTimeout)
	if err != nil {
		return nil, err
	}
	defer func() { _ = l.Release() }() // see Update
	names, err := s.changeNames()
	if err != nil {
		return nil, err
	}
	id := ids.NextChange(names)
	dir := s.WS.ChangeDir(id)
	for _, d := range []string{filepath.Join(dir, "prd"), filepath.Join(dir, "artifacts")} {
		if err := fsutil.EnsureDir(d); err != nil {
			return nil, err
		}
	}
	if err := fsutil.WriteFileAtomic(filepath.Join(dir, "prd", "v1.md"), n.PRD); err != nil {
		return nil, err
	}
	now := s.now()
	c := &Change{
		Schema: SchemaID, ID: id, Title: n.Title, State: Draft,
		CreatedBy: n.CreatedBy, CreatedAt: now, UpdatedAt: now,
		PRD: PRD{Current: "v1", Versions: []PRDVersion{{
			Version: "v1", Path: "prd/v1.md", SHA256: digest.Bytes(n.PRD), Source: n.PRDSource, At: now,
		}}},
		StepAttempts: map[string]int{},
		Budget: Budget{
			MaxCostUSD: n.Budget.MaxCostUSD, MaxHours: n.Budget.MaxHours, MaxToolCalls: n.Budget.MaxToolCalls,
			Status: BudgetOK,
		},
		Approvals: Approvals{},
	}
	if err := Save(s.Reg, s.WS.ChangeFile(id), c); err != nil {
		_ = os.RemoveAll(dir) // never leave a half-created change behind; the save error is reported
		return nil, err
	}
	return c, nil
}

func (s *Store) changeNames() ([]string, error) {
	entries, err := os.ReadDir(s.WS.ChangesDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, aerr.Wrap(aerr.StateNotInitialised, "no changes directory", err)
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "list changes", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
