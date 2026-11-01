// Package audit is Athel's append-only, hash-chained audit log (doc 04 §Audit log, FR-SAFE-011):
// one JSON record per line in .athel/audit/YYYY-MM-DD.jsonl. Every record carries the hash of
// the previous one, so editing, deleting, reordering or truncating any line breaks the chain,
// and `athel audit verify` finds the first break.
//
// Chain rules (decision Q5): hash = sha256 of the record's canonical JSON without its own
// "hash"; seq is global across days; the first record of a day links to the last record of the
// previous file; the very first record links to the all-zero genesis hash.
package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/lock"
)

// SchemaID is the schema of one audit record.
const SchemaID = "athel/audit@1"

// Genesis is prev_hash of the first record ever written.
var Genesis = "sha256:" + strings.Repeat("0", 64)

// MaxAction and MaxReason bound the scrubbed texts stored per record (schema maxLength 2100
// and 2000, with room for the truncation marker).
const (
	MaxAction = 2048
	MaxReason = 1980
)

// Record is one audit line. Field names follow doc 04 (seq, ts, change, step, task, agent, tool,
// action, tier, env, decision, rule, policy_hash, result, prev_hash) plus what M0/M1 need.
// Field order is the canonical order used for hashing.
type Record struct {
	Schema         string  `json:"schema"`
	Seq            int     `json:"seq"`
	TS             string  `json:"ts"`
	Kind           string  `json:"kind"`
	Session        *string `json:"session"`
	PromptID       *string `json:"prompt_id"`
	Change         *string `json:"change"`
	Step           *string `json:"step"`
	Task           *string `json:"task"`
	Agent          *string `json:"agent"`
	Tool           *string `json:"tool"`
	ToolUseID      *string `json:"tool_use_id"`
	Action         *string `json:"action"`
	InputSHA256    *string `json:"input_sha256"`
	Tier           *string `json:"tier"`
	Env            *string `json:"env"`
	Decision       *string `json:"decision"`
	Rule           *string `json:"rule"`
	Code           *string `json:"code"`
	PolicyHash     *string `json:"policy_hash"`
	From           *string `json:"from"`
	To             *string `json:"to"`
	By             *string `json:"by"`
	Reason         *string `json:"reason"`
	IdempotencyKey *string `json:"idempotency_key"`
	DurationUS     *int64  `json:"duration_us"`
	Result         string  `json:"result"`
	PrevHash       string  `json:"prev_hash"`
	Hash           string  `json:"hash"`
}

// Kinds of audit records.
const (
	KindToolDecision      = "tool_decision"
	KindToolCompleted     = "tool_completed"
	KindToolFailed        = "tool_failed"
	KindHook              = "hook"
	KindTransition        = "transition"
	KindIllegalTransition = "illegal_transition"
	KindHumanRequired     = "human_required"
	KindStepEnter         = "step_enter"
	KindStepExit          = "step_exit"
	KindLockTakeover      = "lock_takeover"
	KindGrantCreated      = "grant_created"
	KindGrantUsed         = "grant_used"
	KindIntegrityAccept   = "integrity_accept"
)

// Results.
const (
	ResultOK    = "ok"
	ResultError = "error"
)

// Str returns a pointer to s, or nil for "" (absent fields are null, never "").
func Str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// encode renders a record. A Record holds only strings, ints and pointers to them, so encoding
// cannot fail and the error is deliberately not propagated.
func encode(r Record) []byte {
	b, _ := json.Marshal(r)
	return b
}

// computeHash hashes the canonical JSON of r without its hash.
func computeHash(r Record) string {
	r.Hash = ""
	return digest.Bytes(encode(r))
}

// Log is the audit directory of one workspace.
type Log struct {
	Dir         string
	Clock       clock.Clock
	LockTimeout time.Duration
}

var fileRe = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2})\.jsonl$`)

// files lists the daily files in date order.
func (l *Log) files() ([]string, error) {
	entries, err := os.ReadDir(l.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "list audit files", err)
	}
	var out []string
	for _, e := range entries {
		if fileRe.MatchString(e.Name()) && !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}

// Append stamps r with seq, ts, prev_hash and hash under the audit lock and appends it to the
// file of the current UTC day. It returns the stored record.
func (l *Log) Append(r Record) (Record, error) {
	if err := fsutil.EnsureDir(l.Dir); err != nil {
		return r, err
	}
	fl, err := lock.Acquire(filepath.Join(l.Dir, ".lock"), l.LockTimeout)
	if err != nil {
		return r, err
	}
	defer func() { _ = fl.Release() }() // the append is already durable
	last, err := l.last()
	if err != nil {
		return r, err
	}
	now := l.Clock.Now()
	r.Schema, r.Seq, r.TS, r.PrevHash = SchemaID, 1, clock.Format(now), Genesis
	if last != nil {
		r.Seq, r.PrevHash = last.Seq+1, last.Hash
	}
	r.Action, r.Reason = truncate(r.Action, MaxAction), truncate(r.Reason, MaxReason)
	r.Hash = computeHash(r)
	return r, fsutil.AppendLine(filepath.Join(l.Dir, now.UTC().Format("2006-01-02")+".jsonl"), encode(r))
}

func truncate(s *string, n int) *string {
	if s == nil || len(*s) <= n {
		return s
	}
	cut := (*s)[:n] + "…[truncated]"
	return &cut
}

// last returns the newest record, reading only the tail of the newest file.
func (l *Log) last() (*Record, error) {
	files, err := l.files()
	if err != nil || len(files) == 0 {
		return nil, err
	}
	for i := len(files) - 1; i >= 0; i-- {
		line, err := lastLine(filepath.Join(l.Dir, files[i]))
		if err != nil {
			return nil, err
		}
		if line == nil {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil || r.Hash == "" {
			return nil, aerr.New(aerr.IntegrityAudit, "last audit record in "+files[i]+" is unreadable; run `athel audit verify`")
		}
		return &r, nil
	}
	return nil, nil
}

const tailWindow = 64 * 1024

func lastLine(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // audit file names are matched by fileRe inside the audit dir
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "open "+path, err)
	}
	defer func() { _ = f.Close() }() // read-only
	info, err := f.Stat()
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "stat "+path, err)
	}
	start := max(info.Size()-tailWindow, 0)
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return nil, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	buf = bytes.TrimRight(buf, "\n")
	if len(buf) == 0 {
		return nil, nil
	}
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[i+1:]
	}
	return buf, nil
}

// Break describes the first place the chain fails.
type Break struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// Report is the result of Verify.
type Report struct {
	OK      bool     `json:"ok"`
	Files   []string `json:"files"`
	Records int      `json:"records"`
	LastSeq int      `json:"last_seq"`
	Break   *Break   `json:"break,omitempty"`
}

// Verify re-reads every daily file in order and checks seq continuity, links, hashes, record
// schema and that each record's date matches its file. It never modifies anything.
func (l *Log) Verify() (Report, error) {
	files, err := l.files()
	if err != nil {
		return Report{}, err
	}
	rep := Report{OK: true, Files: files}
	if rep.Files == nil {
		rep.Files = []string{}
	}
	prev, seq := Genesis, 0
	for _, name := range files {
		brk, err := verifyFile(filepath.Join(l.Dir, name), name, &prev, &seq, &rep.Records)
		if err != nil {
			return rep, err
		}
		if brk != nil {
			rep.OK, rep.Break = false, brk
			break
		}
	}
	rep.LastSeq = seq
	return rep, nil
}

func verifyFile(path, name string, prev *string, seq *int, count *int) (*Break, error) {
	f, err := os.Open(path) //nolint:gosec // see lastLine
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "open "+path, err)
	}
	defer func() { _ = f.Close() }() // read-only
	day := strings.TrimSuffix(name, ".jsonl")
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		at := func(reason string) *Break { return &Break{File: name, Line: line, Reason: reason} }
		r, problem := parseRecord(sc.Bytes())
		switch {
		case problem != "":
			return at(problem), nil
		case r.Seq != *seq+1:
			return at(fmt.Sprintf("seq %d follows %d (record missing or reordered)", r.Seq, *seq)), nil
		case r.PrevHash != *prev:
			return at("prev_hash does not match the previous record (record changed, missing or reordered)"), nil
		case !strings.HasPrefix(r.TS, day):
			return at("record dated " + r.TS + " is in the file for " + day), nil
		}
		if computeHash(r) != r.Hash {
			return at("hash does not match the record content (record edited)"), nil
		}
		*prev, *seq = r.Hash, r.Seq
		*count++
	}
	if err := sc.Err(); err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	return nil, nil
}

func parseRecord(b []byte) (Record, string) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var r Record
	if err := dec.Decode(&r); err != nil {
		return r, "not a valid audit record: " + err.Error()
	}
	if r.Schema != SchemaID {
		return r, "unknown schema " + r.Schema
	}
	return r, ""
}

// Tail returns the last n records (or fewer), oldest first.
func (l *Log) Tail(n int) ([]Record, error) {
	files, err := l.files()
	if err != nil {
		return nil, err
	}
	var out []Record
	for i := len(files) - 1; i >= 0 && len(out) < n; i-- {
		recs, err := readFile(filepath.Join(l.Dir, files[i]))
		if err != nil {
			return nil, err
		}
		out = append(recs[max(len(recs)-(n-len(out)), 0):], out...)
	}
	return out, nil
}

// All returns every record, oldest first (tests and small tools).
func (l *Log) All() ([]Record, error) { return l.Tail(int(^uint(0) >> 1)) }

func readFile(path string) ([]Record, error) {
	b, err := os.ReadFile(path) //nolint:gosec // see lastLine
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	var out []Record
	for _, line := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		r, problem := parseRecord(line)
		if problem != "" {
			return nil, aerr.New(aerr.IntegrityAudit, path+": "+problem)
		}
		out = append(out, r)
	}
	return out, nil
}
