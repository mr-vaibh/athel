package facts

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

// Archive moves facts that have been neither verified nor used for ArchiveAfter out of the
// active file into archive/facts.jsonl, keeping every record, so the active file stays small
// (doc 03 §Size and cleanup). It returns the archived IDs.
func (s *Store) Archive(dryRun bool) ([]string, error) {
	out := []string{}
	err := s.locked(func() error {
		recs, raw, err := readRecords(s.factsPath())
		if err != nil {
			return err
		}
		lastUse, err := s.lastUses()
		if err != nil {
			return err
		}
		cutoff := s.Clock.Now().Add(-s.ArchiveAfter)
		gone := map[string]bool{}
		for _, f := range current(recs) {
			idle, err := idleSince(f, lastUse[f.ID], cutoff)
			if err != nil {
				return err
			}
			if idle {
				gone[f.ID] = true
				out = append(out, f.ID)
			}
		}
		if dryRun || len(gone) == 0 {
			return nil
		}
		return s.moveToArchive(recs, raw, gone)
	})
	return out, err
}

// idleSince reports whether the fact's last verification and last use are both before cutoff.
func idleSince(f Fact, lastUse time.Time, cutoff time.Time) (bool, error) {
	at, err := clock.Parse(f.VerifiedAt)
	if err != nil {
		return false, aerr.Wrap(aerr.StateSchema, "fact "+f.ID+" has an invalid verified_at", err)
	}
	return at.Before(cutoff) && lastUse.Before(cutoff), nil
}

// moveToArchive appends the records' lines to the archive first and then rewrites the active
// file: a crash in between leaves a fact in both files, which reads the same, never in neither.
func (s *Store) moveToArchive(recs []Fact, raw [][]byte, gone map[string]bool) error {
	if err := fsutil.EnsureDir(filepath.Dir(s.archivePath())); err != nil {
		return err
	}
	var keep bytes.Buffer
	for i, f := range recs {
		if !gone[f.ID] {
			keep.Write(append(raw[i], '\n'))
			continue
		}
		if err := fsutil.AppendLine(s.archivePath(), raw[i]); err != nil {
			return err
		}
	}
	return fsutil.WriteFileAtomic(s.factsPath(), keep.Bytes())
}

// decodeAccess reads one access-log line; anything else is corruption.
func decodeAccess(line []byte) (access, error) {
	var a access
	if err := json.Unmarshal(line, &a); err != nil {
		return access{}, aerr.Wrap(aerr.StateSchema, "access.jsonl has a line that is not an access record", err)
	}
	return a, nil
}

// lastUses maps each fact to the time of its latest use in the access log.
func (s *Store) lastUses() (map[string]time.Time, error) {
	lines, err := readLines(s.accessPath())
	if err != nil {
		return nil, err
	}
	out := map[string]time.Time{}
	for i, line := range lines {
		a, err := decodeAccess(line)
		if err != nil {
			return nil, err
		}
		at, err := clock.Parse(a.At)
		if err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, s.accessPath()+" line "+strconv.Itoa(i+1)+" has an invalid time", err)
		}
		if at.After(out[a.Fact]) {
			out[a.Fact] = at
		}
	}
	return out, nil
}
