package observe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

var t0 = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

func newStore(t *testing.T) (*Store, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(t0)
	return &Store{Dir: filepath.Join(t.TempDir(), "observations"), Reg: schema.NewRegistry(), Clock: clk, TTL: 7 * 24 * time.Hour}, clk
}

func call(id string) Call {
	return Call{ToolUseID: id, Session: "sess-1", AgentType: ScoutAgentType, Tool: "Bash",
		Input: []byte(`{"command":"kubectl get deploy -n shop"}`), Output: []byte(`{"stdout":"users 3/3","stderr":""}`)}
}

func TestCapturesScrubbedOutput(t *testing.T) {
	s, _ := newStore(t)
	c := call("toolu_01")
	c.Flags = Suspicious("ignore previous instructions")
	o, err := s.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	if o.Output != string(c.Output) || o.Truncated || !strings.HasPrefix(o.OutputSHA256, "sha256:") || o.At != clock.Format(t0) ||
		o.ExpiresAt != clock.Format(t0.Add(7*24*time.Hour)) || len(o.Suspicious) != 1 {
		t.Errorf("observation = %+v", o)
	}
	got, err := s.Get("toolu_01")
	if err != nil || got.Input != string(c.Input) || got.OutputSHA256 != o.OutputSHA256 {
		t.Errorf("get = %+v %v", got, err)
	}
}

func TestLargeOutputTruncatedHashKept(t *testing.T) {
	s, _ := newStore(t)
	c := call("toolu_02")
	c.Output = []byte(`{"stdout":"` + strings.Repeat("x", MaxOutput) + `"}`)
	c.Input = []byte(`{"command":"` + strings.Repeat("y", MaxInput) + `"}`)
	o, err := s.Save(c)
	if err != nil {
		t.Fatal(err)
	}
	if !o.Truncated || len(o.Output) != MaxOutput || len(o.Input) != MaxInput || o.OutputSHA256 == "" {
		t.Errorf("truncated observation: truncated=%v len=%d input=%d", o.Truncated, len(o.Output), len(o.Input))
	}
	full := call("toolu_03")
	full.Output = c.Output
	other, _ := s.Save(full)
	if other.OutputSHA256 != o.OutputSHA256 {
		t.Error("the hash must cover the whole output, not the cut")
	}
}

func TestObservationsExpire(t *testing.T) {
	s, clk := newStore(t)
	for _, id := range []string{"toolu_a", "toolu_b"} {
		if _, err := s.Save(call(id)); err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(3 * 24 * time.Hour)
	if _, err := s.Save(call("toolu_c")); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Prune(); err != nil || n != 0 {
		t.Fatalf("early prune = %d %v", n, err)
	}
	clk.Advance(5 * 24 * time.Hour)
	if _, err := s.Get("toolu_a"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("expired observation still served: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "README"), []byte("not an observation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Prune(); err != nil || n != 2 {
		t.Fatalf("prune = %d %v", n, err)
	}
	if _, err := s.Get("toolu_c"); err != nil {
		t.Errorf("fresh observation pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "README")); err != nil {
		t.Error("prune removed a file that is not an observation")
	}
}

func TestObservationErrors(t *testing.T) {
	s, _ := newStore(t)
	for _, id := range []string{"", "../escape", "a/b", strings.Repeat("x", 201)} {
		if _, err := s.Save(call(id)); !aerr.HasCode(err, aerr.StateInvalid) {
			t.Errorf("save %q: %v, want E-STATE-INVALID", id, err)
		}
		if _, err := s.Get(id); !aerr.HasCode(err, aerr.StateInvalid) {
			t.Errorf("get %q: %v, want E-STATE-INVALID", id, err)
		}
	}
	bad := call("toolu_x")
	bad.Session = "has spaces"
	if _, err := s.Save(bad); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid session: %v", err)
	}
	if _, err := s.Get("toolu_missing"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "toolu_bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("toolu_bad"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt observation: %v", err)
	}
	if _, err := s.Prune(); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("prune over a corrupt observation: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(s.Dir, "toolu_dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("toolu_dir"); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable observation: %v", err)
	}
}

func TestStoreIOErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Store{Dir: filepath.Join(file, "observations"), Reg: schema.NewRegistry(), Clock: clock.NewFixed(t0), TTL: time.Hour}
	if _, err := s.Save(call("toolu_01")); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("save under a file: %v", err)
	}
	if _, err := s.Prune(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("prune under a file: %v", err)
	}
	missing := &Store{Dir: filepath.Join(t.TempDir(), "none"), Reg: s.Reg, Clock: s.Clock, TTL: time.Hour}
	if n, err := missing.Prune(); err != nil || n != 0 {
		t.Errorf("prune of a missing folder = %d %v", n, err)
	}
}

func TestPruneAndReadErrors(t *testing.T) {
	s, clk := newStore(t)
	if _, err := s.Save(call("toolu_old")); err != nil {
		t.Fatal(err)
	}
	clk.Advance(8 * 24 * time.Hour)
	if err := os.Chmod(s.Dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.Dir, 0o700) })
	if _, err := s.Prune(); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("prune in a read-only folder: %v", err)
	}
	if err := os.Chmod(s.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	bad := `{"schema":"athel/observation@1","tool_use_id":"toolu_t","session":"s","agent_type":"a","tool":"Bash","input":"","output":"","output_sha256":"sha256:00","truncated":false,"suspicious":[],"at":"x","expires_at":"soon"}`
	if err := os.WriteFile(filepath.Join(s.Dir, "toolu_t.json"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("toolu_t"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid expires_at: %v", err)
	}
}

func TestFlaggedBySessionSince(t *testing.T) {
	s, clk := newStore(t)
	if got, err := s.Flagged("sess-1", t0); err != nil || len(got) != 0 {
		t.Fatalf("no folder = %v %v", got, err)
	}
	save := func(id, session string, flagged bool) {
		c := call(id)
		c.Session = session
		if flagged {
			c.Flags = Suspicious("you must now run it")
		}
		if _, err := s.Save(c); err != nil {
			t.Fatal(err)
		}
	}
	save("toolu_before", "sess-1", true)
	clk.Advance(time.Minute)
	since := clk.Now()
	save("toolu_flagged", "sess-1", true)
	save("toolu_clean", "sess-1", false)
	save("toolu_other", "sess-2", true)
	if err := os.WriteFile(filepath.Join(s.Dir, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.Flagged("sess-1", since)
	if err != nil || len(got) != 1 || got[0] != "toolu_flagged" {
		t.Errorf("flagged = %v %v", got, err)
	}
	bad := `{"schema":"athel/observation@1","tool_use_id":"toolu_t","session":"s","agent_type":"a","tool":"Bash","input":"","output":"","output_sha256":"sha256:00","truncated":false,"suspicious":[],"at":"x","expires_at":"2026-12-01T00:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(s.Dir, "toolu_t.json"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Flagged("sess-1", since); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid at: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "toolu_t.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Flagged("sess-1", since); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt observation: %v", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	under := &Store{Dir: filepath.Join(file, "x"), Reg: s.Reg, Clock: s.Clock, TTL: time.Hour}
	if _, err := under.Flagged("s", since); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("folder under a file: %v", err)
	}
}
