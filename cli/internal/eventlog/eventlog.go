// Package eventlog appends records to Athel's M0 JSONL logs (.athel/audit/*.jsonl). Each
// record gets a per-file sequence number and a CLI timestamp under an OS lock, so concurrent
// writers never interleave or reuse a number. M1 replaces this with the hash-chained audit
// log; the record fields already follow the doc 04 audit record.
package eventlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/lock"
)

// tailWindow is how much of the file end is read to find the last sequence number. Records
// are far smaller, so the last complete line is always inside it.
const tailWindow = 64 * 1024

// Record is anything that can be stamped with its sequence number and timestamp.
type Record interface {
	Stamp(seq int, ts string)
}

// Log is one append-only JSONL file.
type Log struct {
	Path        string
	Clock       clock.Clock
	LockTimeout time.Duration
}

// Append stamps rec with the next sequence number and the current time and appends it.
func (l *Log) Append(rec Record) error {
	fl, err := lock.Acquire(l.Path+".lock", l.LockTimeout)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Release() }() // the append is already durable
	last, err := lastSeq(l.Path)
	if err != nil {
		return err
	}
	rec.Stamp(last+1, clock.Format(l.Clock.Now()))
	line, err := json.Marshal(rec)
	if err != nil {
		return aerr.Wrap(aerr.StateInvalid, "encode event for "+l.Path, err)
	}
	return fsutil.AppendLine(l.Path, line)
}

// lastSeq returns the "seq" of the last line, or 0 for a missing or empty file.
func lastSeq(path string) (int, error) {
	f, err := os.Open(path) //nolint:gosec // log paths are fixed names inside the workspace
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, aerr.Wrap(aerr.StateIO, "open "+path, err)
	}
	defer func() { _ = f.Close() }() // read-only
	info, err := f.Stat()
	if err != nil {
		return 0, aerr.Wrap(aerr.StateIO, "stat "+path, err)
	}
	start := max(info.Size()-tailWindow, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return 0, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	buf = bytes.TrimRight(buf, "\n")
	if len(buf) == 0 {
		return 0, nil
	}
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[i+1:]
	}
	var probe struct {
		Seq json.Number `json:"seq"`
	}
	if err := json.Unmarshal(buf, &probe); err != nil {
		return 0, aerr.Wrap(aerr.StateIO, "last line of "+path+" is not a JSON record", err)
	}
	seq, err := strconv.Atoi(probe.Seq.String())
	if err != nil || seq < 1 {
		return 0, aerr.New(aerr.StateIO, "last line of "+path+" has no valid seq")
	}
	return seq, nil
}
