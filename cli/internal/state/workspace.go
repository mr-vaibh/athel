package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

// Workspace is a directory that contains .athel/ (doc 02 §State model). Paths that later
// milestones own (knowledge/, lessons/, repos/, locks/) are reserved but not created in M0.
type Workspace struct {
	Root string
}

// Dir is the .athel directory.
func (w Workspace) Dir() string { return filepath.Join(w.Root, ".athel") }

// ConfigDir holds the project config layer.
func (w Workspace) ConfigDir() string { return filepath.Join(w.Dir(), "config") }

// ChangesDir holds one folder per change.
func (w Workspace) ChangesDir() string { return filepath.Join(w.Dir(), "changes") }

// AuditDir holds the append-only event logs.
func (w Workspace) AuditDir() string { return filepath.Join(w.Dir(), "audit") }

// AskDir is the folder of one ad-hoc question (M2 plan G3); id must already be validated.
func (w Workspace) AskDir(id string) string { return filepath.Join(w.Dir(), "asks", id) }

// CacheDir holds Athel's own bare clones (doc 02 §State model: workspace/cache/<repo>.git).
func (w Workspace) CacheDir() string { return filepath.Join(w.Root, "cache") }

// RepoIndexFile is the index of the developer's local clones (doc 03; FR-DISC-015).
func (w Workspace) RepoIndexFile() string { return filepath.Join(w.Dir(), "repos", "index.yaml") }

// KnowledgeDir holds the verified fact cache and its access log (doc 03).
func (w Workspace) KnowledgeDir() string { return filepath.Join(w.Dir(), "knowledge") }

// StateDir holds workspace-wide state written only by the CLI and its hooks (grants).
func (w Workspace) StateDir() string { return filepath.Join(w.Dir(), "state") }

// LockFile serialises workspace-wide operations such as change ID allocation.
func (w Workspace) LockFile() string { return filepath.Join(w.Dir(), ".lock") }

// ChangeDir is the folder of one change; id must already be validated.
func (w Workspace) ChangeDir(id string) string { return filepath.Join(w.ChangesDir(), id) }

// ChangeFile is the change.yaml of one change.
func (w Workspace) ChangeFile(id string) string { return filepath.Join(w.ChangeDir(id), "change.yaml") }

// ChangeLockFile serialises read-modify-write cycles on one change.
func (w Workspace) ChangeLockFile(id string) string { return filepath.Join(w.ChangeDir(id), ".lock") }

// Init creates the M0 part of the .athel tree. It is idempotent and returns the directories
// it created.
func Init(root string) (Workspace, []string, error) {
	w := Workspace{Root: root}
	var created []string
	for _, dir := range []string{w.Dir(), w.ConfigDir(), w.ChangesDir(), w.AuditDir()} {
		if _, err := os.Stat(dir); err == nil {
			continue
		}
		if err := fsutil.EnsureDir(dir); err != nil {
			return w, created, err
		}
		created = append(created, dir)
	}
	return w, created, nil
}

// Find walks up from start to the nearest directory that contains .athel/.
func Find(start string) (Workspace, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return Workspace{}, aerr.Wrap(aerr.StateIO, "resolve "+start, err)
	}
	for {
		info, err := os.Stat(filepath.Join(dir, ".athel"))
		if err == nil && info.IsDir() {
			return Workspace{Root: dir}, nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Workspace{}, aerr.Wrap(aerr.StateIO, "look for .athel in "+dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Workspace{}, aerr.New(aerr.StateNotInitialised, "no .athel directory in "+start+" or any parent").
				WithNextAction("run `athel init` in the workspace root")
		}
		dir = parent
	}
}
