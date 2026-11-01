package hooks

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

const sess = "sess-A"

func newDeps(t *testing.T) (Deps, *clock.Fixed) {
	t.Helper()
	ws, _, err := state.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))
	return Deps{Store: state.NewStore(ws, schema.NewRegistry(), clk), Clock: clk, ConfigHash: "sha256:" + strings.Repeat("a", 64)}, clk
}

func newChange(t *testing.T, d Deps, title string) *state.Change {
	t.Helper()
	c, err := d.Store.Create(state.NewChange{Title: title, CreatedBy: "dev", PRD: []byte("p"), PRDSource: "text",
		Budget: state.Budget{MaxCostUSD: 40, MaxHours: 8, MaxToolCalls: 2000}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func lockFor(t *testing.T, d Deps, id, session string) {
	t.Helper()
	if _, err := d.Store.AcquireLock(id, session, state.AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestPreToolUseStubAllows(t *testing.T) {
	d, _ := newDeps(t)
	for _, tool := range []string{"Bash", "Edit", "Write", "mcp__grafana__query", "WebFetch"} {
		o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: tool, ToolInput: []byte(`{"command":"rm -rf /"}`)}, d)
		if *o.Decision != hookio.Allow || o.Response.Decision != hookio.Allow || o.Rule != RuleStub {
			t.Errorf("%s: %+v", tool, o)
		}
	}
	o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash"}, Deps{})
	if *o.Decision != hookio.Allow {
		t.Errorf("outside a workspace: %+v", o)
	}
}

func TestStubReasonSaysStub(t *testing.T) {
	d, _ := newDeps(t)
	o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash"}, d)
	if !strings.Contains(o.Response.Reason, "M0 STUB") || !strings.Contains(o.Response.Reason, "M1") {
		t.Errorf("reason = %q", o.Response.Reason)
	}
}

func TestPreToolUseHeartbeat(t *testing.T) {
	d, clk := newDeps(t)
	c := newChange(t, d, "x")
	lockFor(t, d, c.ID, sess)
	clk.Advance(6 * time.Minute)
	o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash"}, d)
	got, _ := d.Store.Get(c.ID)
	if o.Change == nil || o.Change.ID != c.ID || got.Lock.HeartbeatAt != "2026-11-01T10:06:00.000Z" || o.Result != "ok" {
		t.Errorf("heartbeat not refreshed: %+v %+v", o, got.Lock)
	}
	other := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: "sess-B", ToolName: "Bash"}, d)
	if other.Change != nil {
		t.Errorf("another session's change attributed: %+v", other.Change)
	}
	if err := os.WriteFile(d.Store.WS.ChangeFile(c.ID), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash"}, d); *o.Decision != hookio.Allow {
		t.Errorf("unreadable state must not block the stub: %+v", o)
	}
}

func TestMalformedDeniesOnlyPreToolUse(t *testing.T) {
	o := Malformed(hookio.PreToolUse, errors.New("bad json"))
	if *o.Decision != hookio.Deny || !strings.Contains(o.Response.Reason, "bad json") || o.Result != "error" {
		t.Errorf("pre-tool-use: %+v", o)
	}
	if o := Malformed(hookio.Stop, errors.New("x")); o.Decision != nil || o.Response.BlockStop {
		t.Errorf("stop: %+v", o)
	}
}

func TestStopHookActiveNeverBlocks(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		red    bool
		count  int
		want   bool
	}{
		{"gates green", false, false, 0, false},
		{"gates red", false, true, 0, true},
		{"red but already continuing", true, true, 0, false},
		{"red at limit", false, true, StopBlockLimit, false},
		{"red below limit", false, true, StopBlockLimit - 1, true},
	}
	for _, tt := range tests {
		if got := ShouldBlockStop(&hookio.Request{StopHookActive: tt.active}, tt.red, tt.count); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
	d, _ := newDeps(t)
	for _, e := range []hookio.Event{hookio.Stop, hookio.SubagentStop} {
		for _, active := range []bool{true, false} {
			if o := Handle(&hookio.Request{Event: e, SessionID: sess, StopHookActive: active}, d); o.Response.BlockStop {
				t.Errorf("M0 %s blocked (active=%v)", e, active)
			}
		}
	}
}

func TestUserPromptSubmitContext(t *testing.T) {
	d, _ := newDeps(t)
	o := Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess, Prompt: "hi"}, d)
	if o.Response.AdditionalContext != "" {
		t.Errorf("no active change but context %q", o.Response.AdditionalContext)
	}
	c := newChange(t, d, "x")
	lockFor(t, d, c.ID, sess)
	o = Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess}, d)
	if o.Response.AdditionalContext != "Athel: active change CHG-001 (draft, no step running)." {
		t.Errorf("context = %q", o.Response.AdditionalContext)
	}
	if _, err := d.Store.EnterStep(c.ID, sess, "start", nil); err != nil {
		t.Fatal(err)
	}
	o = Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess}, d)
	if !strings.Contains(o.Response.AdditionalContext, "step start") {
		t.Errorf("context = %q", o.Response.AdditionalContext)
	}
}

func TestPreCompactCheckpoint(t *testing.T) {
	d, _ := newDeps(t)
	if o := Handle(&hookio.Request{Event: hookio.PreCompact, SessionID: sess, Trigger: "auto"}, d); o.Change != nil {
		t.Errorf("checkpoint without an active change: %+v", o)
	}
	c := newChange(t, d, "x")
	lockFor(t, d, c.ID, sess)
	o := Handle(&hookio.Request{Event: hookio.PreCompact, SessionID: sess, Trigger: "auto"}, d)
	cp := state.LastCheckpoint(o.Change)
	if cp == nil || cp.Name != "precompact" || cp.Trigger != state.TriggerPreCompact || *cp.Note != "context compaction (auto)" {
		t.Errorf("checkpoint = %+v", cp)
	}
	if _, err := d.Store.EnterStep(c.ID, sess, "start", nil); err != nil {
		t.Fatal(err)
	}
	o = Handle(&hookio.Request{Event: hookio.PreCompact, SessionID: sess}, d)
	if cp := state.LastCheckpoint(o.Change); cp.Name != "start.precompact" || *cp.Note != "context compaction (unknown)" {
		t.Errorf("checkpoint in step = %+v", cp)
	}
	if err := os.Chmod(d.Store.WS.ChangeDir(c.ID), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(d.Store.WS.ChangeDir(c.ID), 0o700) })
	if o := Handle(&hookio.Request{Event: hookio.PreCompact, SessionID: sess}, d); o.Result != "error" {
		t.Errorf("unwritable change dir: %+v", o)
	}
}

func TestSessionEndReleasesOnlyOwnLocks(t *testing.T) {
	d, _ := newDeps(t)
	a, b := newChange(t, d, "a"), newChange(t, d, "b")
	lockFor(t, d, a.ID, sess)
	lockFor(t, d, b.ID, "sess-B")
	o := Handle(&hookio.Request{Event: hookio.SessionEnd, SessionID: sess, Reason: "logout"}, d)
	if o.Response.SystemMessage != "Athel released locks: CHG-001" {
		t.Errorf("message = %q", o.Response.SystemMessage)
	}
	ga, _ := d.Store.Get(a.ID)
	gb, _ := d.Store.Get(b.ID)
	if ga.Lock != nil || gb.Lock == nil {
		t.Errorf("locks after session end: a=%v b=%v", ga.Lock, gb.Lock)
	}
	if o := Handle(&hookio.Request{Event: hookio.SessionEnd, SessionID: sess}, Deps{}); o.Result != "ok" {
		t.Errorf("outside a workspace: %+v", o)
	}
	if err := os.WriteFile(d.Store.WS.ChangeFile(b.ID), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o := Handle(&hookio.Request{Event: hookio.SessionEnd, SessionID: sess}, d); o.Result != "error" {
		t.Errorf("unreadable changes: %+v", o)
	}
}

func TestPostToolUseLogsOnly(t *testing.T) {
	d, _ := newDeps(t)
	for _, e := range []hookio.Event{hookio.PostToolUse, hookio.PostToolUseFailure} {
		o := Handle(&hookio.Request{Event: e, SessionID: sess, ToolName: "Read"}, d)
		if o.Rule != RuleLogOnly || o.Decision != nil || o.Response != (hookio.Response{}) {
			t.Errorf("%s: %+v", e, o)
		}
	}
}

func TestOneEventLinePerInvocation(t *testing.T) {
	d, clk := newDeps(t)
	c := newChange(t, d, "x")
	lockFor(t, d, c.ID, sess)
	task := "T-01"
	if _, err := d.Store.Update(c.ID, func(c *state.Change) error {
		c.Step = &state.Step{Name: "start", Task: &task, StartedAt: "2026-11-01T10:00:00.000Z", Attempt: 1, Status: state.StepRunning}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reqs := []*hookio.Request{
		{Event: hookio.PreToolUse, SessionID: sess, PromptID: "p1", ToolName: "Bash", ToolUseID: "toolu_1", ToolInput: []byte(`{"command":"ls"}`), AgentType: "athel:scout"},
		{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p2", Prompt: "approve the plan"},
		{Event: hookio.Stop, SessionID: sess},
	}
	for _, r := range reqs {
		o := Handle(r, d)
		if err := Record(d.Store, NewHookEvent(r.Event, r, o, d.ConfigHash, 812*time.Microsecond)); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Second)
	}
	if err := Record(d.Store, NewHookEvent(hookio.PreToolUse, nil, Malformed(hookio.PreToolUse, errors.New("x")), "", 0)); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, HookEventsFile(d.Store.WS))
	if len(lines) != 4 {
		t.Fatalf("%d lines, want 4", len(lines))
	}
	reg := schema.NewRegistry()
	for i, line := range lines {
		var doc any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatal(err)
		}
		if err := reg.Validate(schema.HookEvent, doc); err != nil {
			t.Errorf("line %d does not match the schema: %v\n%s", i+1, err, line)
		}
	}
	checkToolEvent(t, lines[0])
	checkPromptEvent(t, lines[1])
}

func checkToolEvent(t *testing.T, line string) {
	t.Helper()
	var e HookEvent
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatal(err)
	}
	ok := e.Seq == 1 && *e.Change == "CHG-001" && *e.Step == "start" && *e.Task == "T-01" && *e.Agent == "athel:scout"
	ok = ok && *e.Tool == "Bash" && e.InputSHA256 != nil && *e.Decision == hookio.Allow && e.DurationUS == 812
	if !ok {
		t.Errorf("tool event = %s", line)
	}
}

func checkPromptEvent(t *testing.T, line string) {
	t.Helper()
	var e HookEvent
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "approve the plan") || e.InputSHA256 == nil || *e.PromptID != "p2" || *e.Agent != "main" {
		t.Errorf("prompt event leaks text or misses hash: %s", line)
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}
