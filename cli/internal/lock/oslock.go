//go:build unix

// Package lock provides the two kinds of locks Athel uses: short-lived OS file locks that
// serialise read-modify-write cycles between concurrent CLI processes, and (in session.go)
// the long-lived logical session lock recorded in change.yaml.
package lock

import (
	"errors"
	"os"
	"syscall"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

// pollInterval is how often a blocked Acquire retries. Contention is rare and short
// (a single YAML rewrite), so a small fixed interval beats a wakeup mechanism.
const pollInterval = 5 * time.Millisecond

// FileLock is an exclusive advisory lock on a file, held until Release.
type FileLock struct {
	f *os.File
}

// Acquire takes an exclusive flock on path, creating the file if needed, waiting at most
// timeout. flock locks belong to the open file description, so two handles in one process
// exclude each other exactly like two processes do.
func Acquire(path string, timeout time.Duration) (*FileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fsutil.PrivateFile) //nolint:gosec // lock paths are fixed names inside the workspace
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "open lock file "+path, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &FileLock{f: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close() // the flock error is the one to report
			return nil, aerr.Wrap(aerr.StateIO, "lock "+path, err)
		}
		if time.Now().After(deadline) {
			_ = f.Close() // nothing was locked
			return nil, aerr.New(aerr.StateLockTimeout, "another athel process holds "+path).
				WithNextAction("retry; if it persists, check for a hung athel process")
		}
		time.Sleep(pollInterval)
	}
}

// Release drops the lock. Closing the descriptor releases the flock even if unlock fails.
func (l *FileLock) Release() error {
	uerr := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	cerr := l.f.Close()
	if err := errors.Join(uerr, cerr); err != nil {
		return aerr.Wrap(aerr.StateIO, "release lock "+l.f.Name(), err)
	}
	return nil
}
