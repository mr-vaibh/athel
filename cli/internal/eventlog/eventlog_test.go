package eventlog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

type rec struct {
	Seq  int    `json:"seq"`
	TS   string `json:"ts"`
	Kind string `json:"kind"`
}

func (r *rec) Stamp(seq int, ts string) { r.Seq, r.TS = seq, ts }

type badRec struct{ F func() }

func (*badRec) Stamp(int, string) {}

func newLog(t *testing.T) *Log {
	t.Helper()
	return &Log{
		Path:        filepath.Join(t.TempDir(), "events.jsonl"),
		Clock:       clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)),
		LockTimeout: 5 * time.Second,
	}
}

func readAll(t *testing.T, path string) []rec {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []rec
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var r rec
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

func TestAppendSequencesAndStamps(t *testing.T) {
	l := newLog(t)
	for _, kind := range []string{"a", "b", "c"} {
		if err := l.Append(&rec{Kind: kind}); err != nil {
			t.Fatal(err)
		}
	}
	got := readAll(t, l.Path)
	for i, r := range got {
		if r.Seq != i+1 || r.TS != "2026-11-01T10:00:00.000Z" {
			t.Errorf("record %d = %+v", i, r)
		}
	}
	if len(got) != 3 || got[2].Kind != "c" {
		t.Errorf("records = %+v", got)
	}
}

func TestAppendConcurrentUniqueSeq(t *testing.T) {
	l := newLog(t)
	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Append(&rec{Kind: "x"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got := readAll(t, l.Path)
	if len(got) != n {
		t.Fatalf("%d records, want %d", len(got), n)
	}
	for i, r := range got {
		if r.Seq != i+1 {
			t.Fatalf("record %d has seq %d", i, r.Seq)
		}
	}
}

func TestAppendContinuesLongFile(t *testing.T) {
	l := newLog(t)
	var b strings.Builder
	pad := strings.Repeat("p", 500)
	for i := 1; i <= 300; i++ {
		line, _ := json.Marshal(map[string]any{"seq": i, "pad": pad})
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(l.Path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(&rec{Kind: "next"}); err != nil {
		t.Fatal(err)
	}
	got := readAll(t, l.Path)
	if last := got[len(got)-1]; last.Seq != 301 {
		t.Errorf("last seq = %d, want 301", last.Seq)
	}
}

func TestAppendErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		rec     Record
		code    aerr.Code
	}{
		{"corrupt last line", "{\"seq\":1}\nnot json\n", &rec{}, aerr.StateIO},
		{"last line without seq", "{\"kind\":1}\n", &rec{}, aerr.StateIO},
		{"unencodable record", "", &badRec{F: func() {}}, aerr.StateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLog(t)
			if tt.content != "" {
				if err := os.WriteFile(l.Path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := l.Append(tt.rec); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	l := newLog(t)
	l.Path = filepath.Join(t.TempDir(), "missing", "events.jsonl")
	if err := l.Append(&rec{}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("missing dir: err = %v", err)
	}
	dirLog := newLog(t)
	dirLog.Path = t.TempDir()
	if err := dirLog.Append(&rec{}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("path is a dir: err = %v", err)
	}
}

func TestEmptyFileStartsAtOne(t *testing.T) {
	l := newLog(t)
	if err := os.WriteFile(l.Path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(&rec{}); err != nil {
		t.Fatal(err)
	}
	got := readAll(t, l.Path)
	if got[len(got)-1].Seq != 1 {
		t.Errorf("seq = %d", got[len(got)-1].Seq)
	}
}
