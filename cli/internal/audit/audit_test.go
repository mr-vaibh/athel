package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/lock"
)

func newLog(t *testing.T) (*Log, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC))
	return &Log{Dir: filepath.Join(t.TempDir(), "audit"), Clock: clk, LockTimeout: 5 * time.Second}, clk
}

func appendN(t *testing.T, l *Log, n int) []Record {
	t.Helper()
	var out []Record
	for i := range n {
		r, err := l.Append(Record{Kind: KindToolDecision, Tool: Str("Bash"), Action: Str("ls " + string(rune('a'+i))), Result: ResultOK})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestGenesisAndLinks(t *testing.T) {
	l, _ := newLog(t)
	recs := appendN(t, l, 3)
	if recs[0].PrevHash != Genesis || recs[0].Seq != 1 || recs[0].Schema != SchemaID {
		t.Errorf("first = %+v", recs[0])
	}
	for i := 1; i < 3; i++ {
		if recs[i].PrevHash != recs[i-1].Hash || recs[i].Seq != i+1 {
			t.Errorf("record %d not linked: %+v", i, recs[i])
		}
	}
	rep, err := l.Verify()
	if err != nil || !rep.OK || rep.Records != 3 || rep.LastSeq != 3 {
		t.Errorf("verify = %+v %v", rep, err)
	}
}

func TestChainLinksAcrossDays(t *testing.T) {
	l, clk := newLog(t)
	day1 := appendN(t, l, 2)
	clk.Advance(2 * time.Minute) // past midnight UTC
	day2 := appendN(t, l, 2)
	if day2[0].PrevHash != day1[1].Hash || day2[0].Seq != 3 || !strings.HasPrefix(day2[0].TS, "2026-11-02") {
		t.Errorf("day boundary: %+v", day2[0])
	}
	rep, err := l.Verify()
	if err != nil || !rep.OK || len(rep.Files) != 2 || rep.Records != 4 {
		t.Errorf("verify = %+v %v", rep, err)
	}
	tail, err := l.Tail(3)
	if err != nil || len(tail) != 3 || tail[0].Seq != 2 || tail[2].Seq != 4 {
		t.Errorf("tail = %+v %v", tail, err)
	}
	all, err := l.All()
	if err != nil || len(all) != 4 {
		t.Errorf("all = %d %v", len(all), err)
	}
}

func lines(t *testing.T, l *Log) (string, [][]byte) {
	t.Helper()
	path := filepath.Join(l.Dir, "2026-11-01.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n"))
}

func write(t *testing.T, path string, ls [][]byte) {
	t.Helper()
	if err := os.WriteFile(path, append(bytes.Join(ls, []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		tamper func([][]byte) [][]byte
		want   string
	}{
		{"edit action", func(ls [][]byte) [][]byte {
			ls[1] = bytes.Replace(ls[1], []byte(`"ls b"`), []byte(`"rm -rf /"`), 1)
			return ls
		}, "record edited"},
		{"edit and rehash", func(ls [][]byte) [][]byte {
			ls[1] = bytes.Replace(ls[1], []byte(`"result":"ok"`), []byte(`"result":"error"`), 1)
			return ls
		}, "record edited"},
		{"delete middle", func(ls [][]byte) [][]byte { return append(ls[:1], ls[2:]...) }, "seq 3 follows 1"},
		{"reorder", func(ls [][]byte) [][]byte { ls[1], ls[2] = ls[2], ls[1]; return ls }, "seq 3 follows 1"},
		{"garbage line", func(ls [][]byte) [][]byte { ls[2] = []byte("not json"); return ls }, "not a valid audit record"},
		{"unknown field", func(ls [][]byte) [][]byte {
			ls[0] = bytes.Replace(ls[0], []byte(`{"schema"`), []byte(`{"extra":1,"schema"`), 1)
			return ls
		}, "not a valid audit record"},
		{"wrong schema", func(ls [][]byte) [][]byte {
			ls[0] = bytes.Replace(ls[0], []byte(SchemaID), []byte("athel/audit@9"), 1)
			return ls
		}, "unknown schema"},
		{"forged prev_hash", func(ls [][]byte) [][]byte {
			ls[2] = bytes.Replace(ls[2], []byte(`"prev_hash":"sha256:`), []byte(`"prev_hash":"sha256:f`), 1)
			return ls
		}, "prev_hash does not match"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _ := newLog(t)
			appendN(t, l, 4)
			path, ls := lines(t, l)
			write(t, path, tt.tamper(ls))
			rep, err := l.Verify()
			if err != nil {
				t.Fatal(err)
			}
			if rep.OK || rep.Break == nil || !strings.Contains(rep.Break.Reason, tt.want) {
				t.Errorf("verify = %+v, want break containing %q", rep, tt.want)
			}
		})
	}
}

func TestVerifyDetectsTruncatedAndMovedRecords(t *testing.T) {
	l, clk := newLog(t)
	appendN(t, l, 2)
	clk.Advance(2 * time.Minute)
	appendN(t, l, 1)
	if err := os.Remove(filepath.Join(l.Dir, "2026-11-01.jsonl")); err != nil {
		t.Fatal(err)
	}
	if rep, _ := l.Verify(); rep.OK || !strings.Contains(rep.Break.Reason, "seq 3 follows 0") {
		t.Errorf("deleted day: %+v", rep.Break)
	}
	l2, _ := newLog(t)
	appendN(t, l2, 1)
	src := filepath.Join(l2.Dir, "2026-11-01.jsonl")
	if err := os.Rename(src, filepath.Join(l2.Dir, "2026-10-31.jsonl")); err != nil {
		t.Fatal(err)
	}
	if rep, _ := l2.Verify(); rep.OK || !strings.Contains(rep.Break.Reason, "is in the file for 2026-10-31") {
		t.Errorf("moved file: %+v", rep.Break)
	}
}

func TestAppendRefusesBrokenTail(t *testing.T) {
	l, _ := newLog(t)
	appendN(t, l, 1)
	path, ls := lines(t, l)
	write(t, path, append(ls, []byte("garbage")))
	if _, err := l.Append(Record{Kind: KindHook, Result: ResultOK}); !aerr.HasCode(err, aerr.IntegrityAudit) || aerr.ExitCode(err) != aerr.ExitIntegrity {
		t.Errorf("err = %v", err)
	}
	if _, err := l.Tail(5); !aerr.HasCode(err, aerr.IntegrityAudit) {
		t.Errorf("tail of corrupt file: %v", err)
	}
}

func TestConcurrentAppendsChainIntact(t *testing.T) {
	l, _ := newLog(t)
	var wg sync.WaitGroup
	for range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Append(Record{Kind: KindHook, Result: ResultOK}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if rep, err := l.Verify(); err != nil || !rep.OK || rep.Records != 30 {
		t.Errorf("verify = %+v %v", rep, err)
	}
}

func TestActionTruncatedAndNullFields(t *testing.T) {
	l, _ := newLog(t)
	r, err := l.Append(Record{Kind: KindToolDecision, Action: Str(strings.Repeat("x", MaxAction+10)), Result: ResultOK})
	if err != nil || len(*r.Action) > MaxAction+20 || !strings.HasSuffix(*r.Action, "[truncated]") {
		t.Errorf("action = %d bytes, %v", len(*r.Action), err)
	}
	if Str("") != nil || *Str("a") != "a" {
		t.Error("Str")
	}
	_, ls := lines(t, l)
	if !bytes.Contains(ls[0], []byte(`"session":null`)) {
		t.Errorf("absent fields must be null: %s", ls[0])
	}
}

func TestEmptyAndErrors(t *testing.T) {
	l, _ := newLog(t)
	if rep, err := l.Verify(); err != nil || !rep.OK || rep.Records != 0 || len(rep.Files) != 0 {
		t.Errorf("empty verify = %+v %v", rep, err)
	}
	if tail, err := l.Tail(3); err != nil || len(tail) != 0 {
		t.Errorf("empty tail = %v %v", tail, err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bad := &Log{Dir: filepath.Join(blocker, "audit"), Clock: clock.System{}, LockTimeout: time.Second}
	if _, err := bad.Append(Record{Kind: KindHook}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unwritable dir: %v", err)
	}
	notDir := &Log{Dir: blocker, Clock: clock.System{}}
	if _, err := notDir.Verify(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("dir is a file: %v", err)
	}
	l2, _ := newLog(t)
	appendN(t, l2, 1)
	if err := os.WriteFile(filepath.Join(l2.Dir, "2026-11-02.jsonl"), []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r, err := l2.Append(Record{Kind: KindHook, Result: ResultOK}); err != nil || r.Seq != 2 {
		t.Errorf("empty newest file is skipped: %+v %v", r, err)
	}
}

func TestUnreadableFiles(t *testing.T) {
	l, _ := newLog(t)
	appendN(t, l, 1)
	path := filepath.Join(l.Dir, "2026-11-01.jsonl")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := l.Verify(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("verify unreadable: %v", err)
	}
	if _, err := l.Tail(1); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("tail unreadable: %v", err)
	}
	if _, err := l.Append(Record{Kind: KindHook, Result: ResultOK}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("append after unreadable tail: %v", err)
	}
}

func TestAppendLockTimeout(t *testing.T) {
	l, _ := newLog(t)
	appendN(t, l, 1)
	held, err := lock.Acquire(filepath.Join(l.Dir, ".lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Release() }()
	l.LockTimeout = 20 * time.Millisecond
	if _, err := l.Append(Record{Kind: KindHook, Result: ResultOK}); !aerr.HasCode(err, aerr.StateLockTimeout) {
		t.Errorf("err = %v", err)
	}
}
