// Package fsutil holds the only two ways Athel writes state to disk: whole-file atomic
// replacement and single-line appends. Callers serialise concurrent writers with
// internal/lock; this package guarantees a reader never sees a half-written file.
package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// PrivateFile is the mode for every file Athel writes: state can reveal work in progress.
const PrivateFile fs.FileMode = 0o600

// PrivateDir is the mode for every directory Athel creates.
const PrivateDir fs.FileMode = 0o700

// WriteFileAtomic replaces path with data: write a temp file in the same directory, fsync it,
// rename it over path, then fsync the directory so the rename itself survives a crash.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return aerr.Wrap(aerr.StateIO, "create temp file for "+path, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName) // best effort: the original error is what matters
		}
	}()
	if err := writeSyncClose(tmp, data); err != nil {
		return aerr.Wrap(aerr.StateIO, "write temp file for "+path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return aerr.Wrap(aerr.StateIO, "rename temp file onto "+path, err)
	}
	committed = true
	if err := SyncDir(dir); err != nil {
		return aerr.Wrap(aerr.StateIO, "sync directory of "+path, err)
	}
	return nil
}

func writeSyncClose(f *os.File, data []byte) error {
	_, werr := f.Write(data)
	serr := f.Sync()
	cerr := f.Close()
	return errors.Join(werr, serr, cerr)
}

// AppendLine appends line plus a newline to path in a single write and fsyncs it.
// The file is created if missing. line must not contain a newline: one record, one line.
func AppendLine(path string, line []byte) error {
	for _, b := range line {
		if b == '\n' {
			return aerr.New(aerr.StateInvalid, "append to "+path+": record contains a newline")
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, PrivateFile) //nolint:gosec // path is built by the CLI from the workspace root, not user input
	if err != nil {
		return aerr.Wrap(aerr.StateIO, "open "+path+" for append", err)
	}
	buf := make([]byte, 0, len(line)+1)
	buf = append(append(buf, line...), '\n')
	if err := writeSyncClose(f, buf); err != nil {
		return aerr.Wrap(aerr.StateIO, "append to "+path, err)
	}
	return nil
}

// SyncDir fsyncs a directory so entries created or renamed in it are durable.
func SyncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // dir is derived from a CLI-built path
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

// EnsureDir creates dir and its parents with PrivateDir permissions.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, PrivateDir); err != nil {
		return aerr.Wrap(aerr.StateIO, "create directory "+dir, err)
	}
	return nil
}
