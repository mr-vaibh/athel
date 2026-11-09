package state

import (
	"path/filepath"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

// ChangeFiles are the step outputs the CLI writes into a change folder for its steps; nothing
// else may be written there this way (doc 04: .athel/changes/<id>/ is written only by the CLI).
var ChangeFiles = map[string]bool{"investigation.md": true}

// SaveChangeFile writes one step output into the change folder for the session that holds (or
// takes) the change lock, like every other change write. A finished or cancelled change is
// read-only.
func (s *Store) SaveChangeFile(id, session, name string, data []byte) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if !ChangeFiles[name] {
		return nil, aerr.New(aerr.StateInvalid, name+" is not a change file the CLI writes")
	}
	return s.Update(id, func(c *Change) error {
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; its files cannot change")
		}
		if err := s.claim(c, session, s.Clock.Now()); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(filepath.Join(s.WS.ChangeDir(c.ID), name), data)
	})
}
