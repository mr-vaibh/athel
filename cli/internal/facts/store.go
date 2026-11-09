package facts

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/ids"
	"github.com/mr-vaibh/athel/cli/internal/lock"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// defaultLockTimeout bounds the wait for another athel process holding the cache.
const defaultLockTimeout = 10 * time.Second

// Store is the knowledge cache in one workspace (.athel/knowledge/). Every read and write
// holds an OS file lock, so concurrent CLI processes never see a half-written cache.
type Store struct {
	Dir          string
	Reg          *schema.Registry
	Clock        clock.Clock
	Windows      map[string]time.Duration // freshness window per class (config facts.freshness)
	ArchiveAfter time.Duration            // config facts.archive_after_days
	LockTimeout  time.Duration
}

// View is a fact with its freshness at the time it was read.
type View struct {
	Fact
	AgeS    int64 `json:"age_s"`
	WindowS int64 `json:"window_s"`
	Fresh   bool  `json:"fresh"` // active and verified within its window
}

// Query selects facts; empty fields match anything. Without All, superseded facts are left out.
type Query struct {
	SubjectKind, Subject, Predicate, Object, Env string
	All                                          bool
}

func (s *Store) factsPath() string   { return filepath.Join(s.Dir, "facts.jsonl") }
func (s *Store) accessPath() string  { return filepath.Join(s.Dir, "access.jsonl") }
func (s *Store) archivePath() string { return filepath.Join(s.Dir, "archive", "facts.jsonl") }

func (s *Store) locked(fn func() error) error {
	if err := fsutil.EnsureDir(s.Dir); err != nil {
		return err
	}
	timeout := s.LockTimeout
	if timeout == 0 {
		timeout = defaultLockTimeout
	}
	fl, err := lock.Acquire(filepath.Join(s.Dir, ".lock"), timeout)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Release() }() // every write below is already durable
	return fn()
}

// Add stores a new fact, or refreshes the active fact with the same subject, predicate, object
// and environment (doc 03 verification rule 5). It reports whether a new fact was created. A new
// fact with the key of a disputed one supersedes it (doc 03 rule 4): the old fact is kept, marked
// superseded, and the new one names it.
func (s *Store) Add(d Draft) (Fact, bool, error) {
	if len(d.Sources) == 0 || len(d.Verify) == 0 {
		return Fact{}, false, aerr.New(aerr.StateFactNoSource, "a fact needs at least one source and a verification recipe").
			WithNextAction("add the tool references the fact came from and the read-only steps that re-check it")
	}
	var out Fact
	created := false
	err := s.locked(func() error {
		recs, archived, err := s.readAll()
		if err != nil {
			return err
		}
		f, now, cur := d.fact(), clock.Format(s.Clock.Now()), current(recs)
		var replaced *Fact
		if old := findActive(cur, f); old != nil {
			f.ID, f.CreatedAt, f.Supersedes = old.ID, old.CreatedAt, old.Supersedes
		} else {
			f.ID, f.CreatedAt, created = ids.Fact.Next(allIDs(recs, archived)), now, true
			if replaced = findDisputed(cur, f); replaced != nil {
				f.Supersedes = &replaced.ID
			}
		}
		f.VerifiedAt, f.Status = now, Active
		lines, err := s.encodeAll(f, replaced)
		if err != nil {
			return err
		}
		out = f
		for _, b := range lines {
			if err := fsutil.AppendLine(s.factsPath(), b); err != nil {
				return err
			}
		}
		return nil
	})
	return out, created, err
}

// Get returns the current record of a fact and whether it has been archived.
func (s *Store) Get(id string) (Fact, bool, error) {
	if _, err := ids.Fact.Parse(id); err != nil {
		return Fact{}, false, err
	}
	var out Fact
	archived := false
	err := s.locked(func() error {
		recs, arch, err := s.readAll()
		if err != nil {
			return err
		}
		if f := lookup(current(recs), id); f != nil {
			out = *f
			return nil
		}
		if f := lookup(current(arch), id); f != nil {
			out, archived = *f, true
			return nil
		}
		return aerr.New(aerr.StateNotFound, "no fact "+id)
	})
	return out, archived, err
}

// Find returns the current facts matching q, oldest first, with their freshness.
func (s *Store) Find(q Query) ([]View, error) {
	out := []View{}
	err := s.locked(func() error {
		recs, _, err := readRecords(s.factsPath())
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		for _, f := range current(recs) {
			if !q.matches(f) {
				continue
			}
			v, err := s.view(f, now)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func (q Query) matches(f Fact) bool {
	return (q.All || f.Status != Superseded) &&
		(q.SubjectKind == "" || f.Subject["kind"] == q.SubjectKind) &&
		(q.Subject == "" || f.Subject["name"] == q.Subject) &&
		(q.Predicate == "" || f.Predicate == q.Predicate) &&
		(q.Object == "" || f.Object["name"] == q.Object) &&
		(q.Env == "" || f.Env == q.Env)
}

// view computes a fact's age and freshness. A volatile fact (window 0) is never fresh: it is
// always re-verified before a decision.
func (s *Store) view(f Fact, now time.Time) (View, error) {
	at, err := clock.Parse(f.VerifiedAt)
	if err != nil {
		return View{}, aerr.Wrap(aerr.StateSchema, "fact "+f.ID+" has an invalid verified_at", err)
	}
	age, window := now.Sub(at), s.Windows[f.FreshnessClass]
	return View{
		Fact: f, AgeS: int64(age / time.Second), WindowS: int64(window / time.Second),
		Fresh: f.Status == Active && window > 0 && age <= window,
	}, nil
}

func (s *Store) readAll() (recs, archived []Fact, err error) {
	if recs, _, err = readRecords(s.factsPath()); err != nil {
		return nil, nil, err
	}
	archived, _, err = readRecords(s.archivePath())
	return recs, archived, err
}

// readRecords reads every record of a facts file in order, with each record's raw line. A line
// that is not a fact@1 record fails closed: only the CLI writes the cache, so anything else is
// corruption.
func readRecords(path string) ([]Fact, [][]byte, error) {
	lines, err := readLines(path)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Fact, 0, len(lines))
	for i, line := range lines {
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var f Fact
		if err := dec.Decode(&f); err != nil || f.Schema != SchemaID {
			return nil, nil, aerr.Wrap(aerr.StateSchema, fmt.Sprintf("%s line %d is not an %s record", path, i+1, SchemaID), err)
		}
		out = append(out, f)
	}
	return out, lines, nil
}

// maxLine bounds one JSONL record; a longer line is corruption, not data.
const maxLine = 1 << 20

// readLines returns the lines of a JSONL file; a missing file has none.
func readLines(path string) ([][]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a fixed file inside the workspace's knowledge folder
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	var lines [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), maxLine)
	for sc.Scan() {
		lines = append(lines, bytes.Clone(sc.Bytes()))
	}
	if err := sc.Err(); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, path+" has a line longer than 1 MiB", err)
	}
	return lines, nil
}

// current folds the record log into the latest record of each fact, in first-seen order.
func current(recs []Fact) []Fact {
	index := map[string]int{}
	out := []Fact{}
	for _, f := range recs {
		if i, ok := index[f.ID]; ok {
			out[i] = f
			continue
		}
		index[f.ID] = len(out)
		out = append(out, f)
	}
	return out
}

func lookup(facts []Fact, id string) *Fact {
	for i := range facts {
		if facts[i].ID == id {
			return &facts[i]
		}
	}
	return nil
}

// encodeAll validates the new record and, when it replaces a disputed fact, that fact's
// superseded record, before anything is written; the superseded record goes first.
func (s *Store) encodeAll(f Fact, replaced *Fact) ([][]byte, error) {
	b, err := s.encode(SchemaID, f)
	if err != nil || replaced == nil {
		return [][]byte{b}, err
	}
	old := *replaced
	old.Status = Superseded
	ob, err := s.encode(SchemaID, old)
	return [][]byte{ob, b}, err
}

// findDisputed returns the disputed fact with f's key, which a new fact supersedes.
func findDisputed(facts []Fact, f Fact) *Fact {
	k := f.key()
	for i := range facts {
		if facts[i].Status == Disputed && facts[i].key() == k {
			return &facts[i]
		}
	}
	return nil
}

func findActive(facts []Fact, f Fact) *Fact {
	k := f.key()
	for i := range facts {
		if facts[i].Status == Active && facts[i].key() == k {
			return &facts[i]
		}
	}
	return nil
}

// allIDs lists every ID ever used, archived ones included, so an ID is never reused.
func allIDs(groups ...[]Fact) []string {
	var out []string
	for _, g := range groups {
		for _, f := range g {
			out = append(out, f.ID)
		}
	}
	return out
}

// encode renders one record and validates it against its schema, so nothing invalid is written.
func (s *Store) encode(id string, v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "encode "+id+" record", err)
	}
	return b, s.Reg.ValidateYAML(id, b)
}
