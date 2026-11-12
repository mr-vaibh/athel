package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

// changeFiles are the step outputs the CLI writes into a change folder for its steps; nothing
// else may be written there this way (doc 04: .athel/changes/<id>/ is written only by the CLI).
var changeFiles = regexp.MustCompile(`^(investigation\.md|blast-radius\.md|decisions\.yaml|traceability\.md|blast-signoff\.yaml|spec/[A-Za-z0-9._-]{1,100}\.md|prd/v[1-9][0-9]*\.index\.yaml|prd/review-v[1-9][0-9]*\.md)$`)

// IsChangeFile reports whether name is a step output the CLI may write into a change folder.
func IsChangeFile(name string) bool { return changeFiles.MatchString(name) }

// SaveChangeFile writes one step output into the change folder for the session that holds (or
// takes) the change lock, like every other change write. A finished or cancelled change is
// read-only.
func (s *Store) SaveChangeFile(id, session, name string, data []byte) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if !IsChangeFile(name) {
		return nil, aerr.New(aerr.StateInvalid, name+" is not a change file the CLI writes")
	}
	return s.Update(id, func(c *Change) error {
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; its files cannot change")
		}
		if err := s.claim(c, session, s.Clock.Now()); err != nil {
			return err
		}
		path := filepath.Join(s.WS.ChangeDir(c.ID), name)
		if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, data)
	})
}

// UpdateChangeFile reads a change file (nil when it does not exist yet), lets fn compute its new
// content, and writes it, all under the change lock, so two sessions never overwrite each
// other's edits (decisions.yaml is read, changed and written back).
func (s *Store) UpdateChangeFile(id, session, name string, fn func(old []byte) ([]byte, error)) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if !IsChangeFile(name) {
		return nil, aerr.New(aerr.StateInvalid, name+" is not a change file the CLI writes")
	}
	return s.Update(id, func(c *Change) error {
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; its files cannot change")
		}
		if err := s.claim(c, session, s.Clock.Now()); err != nil {
			return err
		}
		path := filepath.Join(s.WS.ChangeDir(c.ID), name)
		old, err := os.ReadFile(path) //nolint:gosec // a change file named by the allow-list
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return aerr.Wrap(aerr.StateIO, "read "+name, err)
		}
		data, err := fn(old)
		if err != nil {
			return err
		}
		if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, data)
	})
}

// AddPRDVersion stores the next PRD version (doc 02: prd/v2.md …) with its index and makes it
// current, under the change lock. want is the version the caller computed the index for; if
// another version landed first, nothing is written (E-CONFLICT) so the index never mismatches.
func (s *Store) AddPRDVersion(id, session, want string, text []byte, source string, index []byte) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	return s.Update(id, func(c *Change) error {
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; its PRD cannot change")
		}
		if next := NextPRDVersion(c); next != want {
			return aerr.New(aerr.ConflictVersion, "the next PRD version of "+c.ID+" is "+next+", not "+want).
				WithNextAction("run the command again")
		}
		if err := s.claim(c, session, s.Clock.Now()); err != nil {
			return err
		}
		dir := filepath.Join(s.WS.ChangeDir(c.ID), "prd")
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, want+".md"), text); err != nil {
			return err
		}
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, want+".index.yaml"), index); err != nil {
			return err
		}
		now := s.now()
		c.PRD.Versions = append(c.PRD.Versions, PRDVersion{Version: want, Path: "prd/" + want + ".md",
			SHA256: digest.Bytes(text), Source: source, At: now})
		c.PRD.Current = want
		return nil
	})
}

// NextPRDVersion is the version a new PRD would get.
func NextPRDVersion(c *Change) string { return "v" + strconv.Itoa(len(c.PRD.Versions)+1) }
