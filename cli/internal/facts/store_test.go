package facts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

var t0 = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

// windows are doc 03's freshness classes, as the built-in config sets them.
var windows = map[string]time.Duration{
	Volatile: 0, Runtime: time.Hour, Topology: 24 * time.Hour, Structural: 7 * 24 * time.Hour, Organisational: 7 * 24 * time.Hour,
}

func newStore(t testing.TB) (*Store, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(t0)
	return &Store{Dir: filepath.Join(t.TempDir(), "knowledge"), Reg: schema.NewRegistry(), Clock: clk,
		Windows: windows, ArchiveAfter: 90 * 24 * time.Hour, LockTimeout: 5 * time.Second}, clk
}

const draftYAML = `subject: {kind: event, name: UserUpdated, topic: users.updated.v1}
predicate: consumed_by
object: {kind: service, name: crm-sync}
env: prod
confidence: confirmed
sources:
  - {kind: code, ref: crm-sync/src/consumers/user.ts:14, commit: 9f3c2ab}
  - {kind: metric, ref: "grafana:kafka-consumer-lag?group=crm-sync", window: 7d, tool_use_id: toolu_01}
verify:
  - {tool: git, action: grep, repo: crm-sync, pattern: users.updated.v1}
freshness_class: topology
derived_from: [toolu_01]
`

func draft(t testing.TB, mutate ...func(*Draft)) Draft {
	t.Helper()
	d, err := DecodeDraft([]byte(draftYAML))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mutate {
		m(&d)
	}
	return d
}

func mustAdd(t testing.TB, s *Store, d Draft) Fact {
	t.Helper()
	f, _, err := s.Add(d)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestAddAssignsCLIFields(t *testing.T) {
	s, _ := newStore(t)
	f, created, err := s.Add(draft(t))
	if err != nil || !created {
		t.Fatalf("add = %v created=%v", err, created)
	}
	want := clock.Format(t0)
	if f.ID != "F-0001" || f.Schema != SchemaID || f.Status != Active || f.CreatedAt != want || f.VerifiedAt != want || f.Supersedes != nil {
		t.Errorf("fact = %+v", f)
	}
	got, archived, err := s.Get("F-0001")
	if err != nil || archived || got.Subject["topic"] != "users.updated.v1" || got.Sources[1].ToolUseID != "toolu_01" {
		t.Errorf("get = %+v archived=%v %v", got, archived, err)
	}
	second := mustAdd(t, s, draft(t, func(d *Draft) { d.Object = Entity{"kind": "service", "name": "notification-service"} }))
	if second.ID != "F-0002" {
		t.Errorf("second id = %s", second.ID)
	}
}

func TestRefusesNoSourceOrRecipe(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Draft)
		code   aerr.Code
	}{
		{"no sources", func(d *Draft) { d.Sources = nil }, aerr.StateFactNoSource},
		{"no recipe", func(d *Draft) { d.Verify = nil }, aerr.StateFactNoSource},
		{"unknown predicate", func(d *Draft) { d.Predicate = "likes" }, aerr.StateSchema},
		{"free-text source", func(d *Draft) { d.Sources[0].Ref = "" }, aerr.StateSchema},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newStore(t)
			if _, _, err := s.Add(draft(t, tt.mutate)); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
			if _, err := os.Stat(filepath.Join(s.Dir, "facts.jsonl")); err == nil {
				t.Error("a refused fact was written")
			}
		})
	}
}

func TestDecodeDraftRejectsCLIFields(t *testing.T) {
	for _, extra := range []string{"id: F-0009\n", "status: active\n", "verified_at: \"2026-01-01T00:00:00.000Z\"\n", "bogus: 1\n"} {
		if _, err := DecodeDraft([]byte(draftYAML + extra)); !aerr.HasCode(err, aerr.StateInvalid) {
			t.Errorf("%q: err = %v, want E-STATE-INVALID", extra, err)
		}
	}
	if _, err := DecodeDraft([]byte(draftYAML + "---\nsubject: {}\n")); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("two documents: %v", err)
	}
}

func TestDedupeRefreshes(t *testing.T) {
	s, clk := newStore(t)
	first := mustAdd(t, s, draft(t))
	clk.Advance(2 * time.Hour)
	again, created, err := s.Add(draft(t, func(d *Draft) { d.Confidence = "likely" }))
	if err != nil || created {
		t.Fatalf("re-add = %v created=%v", err, created)
	}
	if again.ID != first.ID || again.CreatedAt != first.CreatedAt || again.VerifiedAt != clock.Format(t0.Add(2*time.Hour)) || again.Confidence != "likely" {
		t.Errorf("refreshed = %+v", again)
	}
	other := mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "staging" }))
	if other.ID == first.ID {
		t.Error("a different environment was merged into the same fact")
	}
	all, err := s.Find(Query{All: true})
	if err != nil || len(all) != 2 {
		t.Errorf("find all = %d facts, %v", len(all), err)
	}
}

func TestFreshnessWindows(t *testing.T) {
	tests := []struct {
		class string
		age   time.Duration
		fresh bool
	}{
		{Volatile, 0, false},
		{Runtime, 59 * time.Minute, true},
		{Runtime, 61 * time.Minute, false},
		{Topology, 24 * time.Hour, true},
		{Topology, 24*time.Hour + time.Second, false},
		{Structural, 6 * 24 * time.Hour, true},
		{Organisational, 8 * 24 * time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.class, tt.age), func(t *testing.T) {
			s, clk := newStore(t)
			f := mustAdd(t, s, draft(t, func(d *Draft) { d.FreshnessClass = tt.class }))
			clk.Advance(tt.age)
			v, err := s.Find(Query{Subject: "UserUpdated"})
			if err != nil || len(v) != 1 {
				t.Fatalf("find = %v %v", v, err)
			}
			if v[0].ID != f.ID || v[0].Fresh != tt.fresh || v[0].AgeS != int64(tt.age/time.Second) || v[0].WindowS != int64(windows[tt.class]/time.Second) {
				t.Errorf("view = %+v", v[0])
			}
		})
	}
}

func TestFindFilters(t *testing.T) {
	s, _ := newStore(t)
	mustAdd(t, s, draft(t))
	mustAdd(t, s, draft(t, func(d *Draft) {
		d.Subject, d.Predicate, d.Object = Entity{"kind": "service", "name": "users"}, "owned_by", Entity{"kind": "team", "name": "identity"}
		d.Env, d.FreshnessClass = "any", Organisational
	}))
	tests := []struct {
		q    Query
		want int
	}{
		{Query{}, 2},
		{Query{SubjectKind: "service"}, 1},
		{Query{Subject: "UserUpdated", Predicate: "consumed_by"}, 1},
		{Query{Object: "identity"}, 1},
		{Query{Env: "prod"}, 1},
		{Query{Env: "staging"}, 0},
		{Query{Predicate: "calls"}, 0},
	}
	for _, tt := range tests {
		if v, err := s.Find(tt.q); err != nil || len(v) != tt.want {
			t.Errorf("find %+v = %d, %v; want %d", tt.q, len(v), err, tt.want)
		}
	}
}

func TestUseLogsEveryAccess(t *testing.T) {
	s, clk := newStore(t)
	f := mustAdd(t, s, draft(t))
	r, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "sess-1")
	if err != nil || r.Result != InWindow {
		t.Fatalf("fresh decision use = %+v %v", r, err)
	}
	clk.Advance(48 * time.Hour)
	if r, err := s.Use(f.ID, Orientation, Ref{Ask: "ASK-001"}, ""); err != nil || r.Result != Unverified || r.AgeS != 48*3600 {
		t.Errorf("stale orientation use = %+v %v", r, err)
	}
	recs := readAccess(t, s)
	if len(recs) != 2 || !strings.Contains(recs[0], `"change":"CHG-001"`) || !strings.Contains(recs[0], `"result":"in_window"`) ||
		!strings.Contains(recs[1], `"ask":"ASK-001"`) || !strings.Contains(recs[1], `"session":null`) || !strings.Contains(recs[1], `"age_s":172800`) {
		t.Errorf("access log = %v", recs)
	}
	for _, line := range recs {
		if err := s.Reg.ValidateYAML(AccessSchemaID, []byte(line)); err != nil {
			t.Errorf("access record invalid: %v", err)
		}
	}
}

func TestDecisionUseOfStaleFactFails(t *testing.T) {
	s, clk := newStore(t)
	f := mustAdd(t, s, draft(t))
	clk.Advance(25 * time.Hour)
	r, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "sess-1")
	if !aerr.HasCode(err, aerr.StateFactStale) || r.Result != RefusedStale {
		t.Fatalf("stale decision use = %+v %v", r, err)
	}
	if recs := readAccess(t, s); len(recs) != 1 || !strings.Contains(recs[0], `"result":"refused_stale"`) {
		t.Errorf("refused use not logged: %v", recs)
	}
	v := mustAdd(t, s, draft(t, func(d *Draft) { d.FreshnessClass = Volatile }))
	if _, err := s.Use(v.ID, Decision, Ref{Change: "CHG-001"}, "sess-1"); !aerr.HasCode(err, aerr.StateFactStale) {
		t.Errorf("volatile fact used for a decision without verification: %v", err)
	}
}

func TestUseErrors(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	tests := []struct {
		name, id, purpose string
		ref               Ref
		code              aerr.Code
	}{
		{"bad id", "F-1", Decision, Ref{Change: "CHG-001"}, aerr.StateInvalid},
		{"unknown fact", "F-0099", Decision, Ref{Change: "CHG-001"}, aerr.StateNotFound},
		{"bad purpose", f.ID, "curiosity", Ref{Change: "CHG-001"}, aerr.StateInvalid},
		{"no ref", f.ID, Decision, Ref{}, aerr.StateInvalid},
		{"both refs", f.ID, Decision, Ref{Change: "CHG-001", Ask: "ASK-001"}, aerr.StateInvalid},
		{"bad change", f.ID, Decision, Ref{Change: "CHG-1"}, aerr.StateInvalid},
		{"bad ask", f.ID, Decision, Ref{Ask: "ASK-1"}, aerr.StateInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.Use(tt.id, tt.purpose, tt.ref, "s"); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestDisputedFactNotUsedForDecisions(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	f.Status = Disputed
	appendRaw(t, s, f)
	if r, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "s"); !aerr.HasCode(err, aerr.StateFactDisputed) || r.Result != RefusedDisputed {
		t.Errorf("disputed decision use = %+v %v", r, err)
	}
	if r, err := s.Use(f.ID, Orientation, Ref{Change: "CHG-001"}, "s"); err != nil || r.Result != Unverified || r.Status != Disputed {
		t.Errorf("disputed orientation use = %+v %v", r, err)
	}
	if v, _ := s.Find(Query{}); len(v) != 1 || v[0].Fresh {
		t.Errorf("a disputed fact is never fresh: %+v", v)
	}
}

func TestArchiveAfter90Days(t *testing.T) {
	s, clk := newStore(t)
	old := mustAdd(t, s, draft(t))
	used := mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "staging" }))
	clk.Advance(60 * 24 * time.Hour)
	if _, err := s.Use(used.ID, Orientation, Ref{Ask: "ASK-001"}, "s"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(31 * 24 * time.Hour)
	got, err := s.Archive(true)
	if err != nil || len(got) != 1 || got[0] != old.ID {
		t.Fatalf("dry run = %v %v", got, err)
	}
	if v, _ := s.Find(Query{}); len(v) != 2 {
		t.Fatal("dry run archived")
	}
	if got, err := s.Archive(false); err != nil || len(got) != 1 {
		t.Fatalf("archive = %v %v", got, err)
	}
	if v, _ := s.Find(Query{}); len(v) != 1 || v[0].ID != used.ID {
		t.Errorf("active facts after archive = %+v", v)
	}
}

func TestArchivedFactsStayReadableNeverReused(t *testing.T) {
	s, clk := newStore(t)
	old := mustAdd(t, s, draft(t))
	mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "staging" }))
	clk.Advance(91 * 24 * time.Hour)
	if got, err := s.Archive(false); err != nil || len(got) != 2 {
		t.Fatalf("archive = %v %v", got, err)
	}
	f, archived, err := s.Get(old.ID)
	if err != nil || !archived || f.ID != old.ID {
		t.Errorf("archived fact = %+v %v %v", f, archived, err)
	}
	if _, err := s.Use(old.ID, Orientation, Ref{Ask: "ASK-001"}, "s"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("use of an archived fact: %v", err)
	}
	if next := mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "dev" })); next.ID != "F-0003" {
		t.Errorf("archived IDs must never be reused: got %s", next.ID)
	}
	if got, _ := s.Archive(false); len(got) != 0 {
		t.Errorf("second archive = %v", got)
	}
}

func TestConcurrentAdds(t *testing.T) {
	s, _ := newStore(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := *s // a separate store per goroutine, as separate processes would have
			_, _, err := other.Add(draft(t, func(d *Draft) { d.Object = Entity{"kind": "service", "name": fmt.Sprintf("svc-%02d", i)} }))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.Find(Query{})
	if err != nil || len(v) != n {
		t.Fatalf("facts = %d %v", len(v), err)
	}
	seen := map[string]bool{}
	for _, f := range v {
		if seen[f.ID] {
			t.Errorf("duplicate id %s", f.ID)
		}
		seen[f.ID] = true
	}
	if !seen["F-0001"] || !seen[fmt.Sprintf("F-%04d", n)] {
		t.Errorf("ids not dense: %v", seen)
	}
}

func TestCorruptCacheFailsClosed(t *testing.T) {
	tests := map[string]string{
		"not json":        "{\n",
		"unknown field":   `{"schema":"athel/fact@1","bogus":1}` + "\n",
		"wrong schema":    `{"schema":"athel/fact@9"}` + "\n",
		"bad verified_at": "",
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			s, _ := newStore(t)
			f := mustAdd(t, s, draft(t))
			if line == "" {
				f.VerifiedAt = "yesterday"
				appendRaw(t, s, f)
			} else {
				appendLine(t, s, line)
			}
			if _, err := s.Find(Query{}); !aerr.HasCode(err, aerr.StateSchema) {
				t.Errorf("err = %v, want E-STATE-SCHEMA", err)
			}
		})
	}
}

func TestStoreIOErrors(t *testing.T) {
	s, _ := newStore(t)
	if err := os.MkdirAll(filepath.Join(s.Dir, "facts.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(Query{}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable cache: %v", err)
	}
	if _, _, err := s.Add(draft(t)); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("add to an unreadable cache: %v", err)
	}
	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := &Store{Dir: filepath.Join(file, "knowledge"), Reg: s.Reg, Clock: s.Clock, Windows: windows}
	if _, _, err := blocked.Add(draft(t)); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("cache dir under a file: %v", err)
	}
}

func BenchmarkFind10k(b *testing.B) {
	s, _ := newStore(b)
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		b.Fatal(err)
	}
	f := mustAdd(b, s, draft(b))
	var sb strings.Builder
	for i := 2; i <= 10000; i++ {
		f.ID, f.Object = fmt.Sprintf("F-%04d", i), Entity{"kind": "service", "name": fmt.Sprintf("svc-%d", i)}
		sb.Write(encodeLine(b, f))
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "facts.jsonl"), []byte(sb.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if v, err := s.Find(Query{Object: "svc-9999"}); err != nil || len(v) != 1 {
			b.Fatalf("find = %d %v", len(v), err)
		}
	}
}
