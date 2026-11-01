package hooks

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/engine"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/scrub"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// fixedDecision makes Decide return dec for every call, echoing the command as the action.
func fixedDecision(dec engine.Decision, rule string) func(*hookio.Request) engine.Result {
	return func(req *hookio.Request) engine.Result {
		return engine.Result{Decision: dec, Rule: rule, Tier: classify.R0, Env: "n/a", Reason: "why " + string(req.ToolInput),
			Actions: []engine.ActionDecision{{Action: classify.Action{Text: string(req.ToolInput)}, Decision: dec, Rule: rule}}}
	}
}

const sess = "sess-A"

func newDeps(t *testing.T) (Deps, *clock.Fixed) {
	t.Helper()
	ws, _, err := state.Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))
	return Deps{Store: state.NewStore(ws, schema.NewRegistry(), clk), Clock: clk, ConfigHash: "sha256:" + strings.Repeat("a", 64),
		Decide: fixedDecision(engine.Allow, "12"), Scrub: scrub.New()}, clk
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

func TestPreToolUseUsesTheEngine(t *testing.T) {
	tests := []struct {
		dec  engine.Decision
		want hookio.Decision
	}{
		{engine.Allow, hookio.Allow}, {engine.Approve, hookio.Approve}, {engine.Deny, hookio.Deny}, {engine.None, ""},
	}
	for _, tt := range tests {
		d, _ := newDeps(t)
		d.Decide = fixedDecision(tt.dec, "3")
		o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash", ToolInput: []byte(`{"command":"ls"}`)}, d)
		if o.Response.Decision != tt.want || o.Rule != "3" || o.Policy == nil || o.Policy.Decision != tt.dec {
			t.Errorf("%s: %+v", tt.dec, o)
		}
		if (o.Decision == nil) != (tt.want == "") || (tt.want != "" && *o.Decision != tt.want) {
			t.Errorf("%s: outcome decision %v", tt.dec, o.Decision)
		}
		rec := NewAuditRecord(hookio.PreToolUse, &hookio.Request{SessionID: sess}, o, "", 0)
		if *rec.Decision != string(tt.dec) || *rec.Tier != "R0" || *rec.Env != "n/a" || *rec.Rule != "3" {
			t.Errorf("%s: audit %+v", tt.dec, rec)
		}
	}
}

func TestPreToolUseScrubsTheAuditedAction(t *testing.T) {
	d, _ := newDeps(t)
	d.Decide = fixedDecision(engine.Deny, "3")
	token := "ghp_" + strings.Repeat("A", 36)
	o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash", ToolInput: []byte(`curl -H "Authorization: token ` + token + `"`)}, d)
	if strings.Contains(o.Action, token) || strings.Contains(o.Reason, token) || !strings.Contains(o.Action, "[REDACTED:") {
		t.Errorf("secret reached the audit fields: action %q reason %q", o.Action, o.Reason)
	}
	if !strings.Contains(o.Response.Reason, "why") {
		t.Errorf("reason to Claude: %q", o.Response.Reason)
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
	if o := Handle(&hookio.Request{Event: hookio.PreToolUse, SessionID: sess, ToolName: "Bash"}, d); o.Policy == nil {
		t.Errorf("unreadable state must still reach the engine: %+v", o)
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
		o := Handle(&hookio.Request{Event: e, SessionID: sess, ToolName: "Read", ToolResponse: []byte(`{"stdout":"hello"}`)}, d)
		if o.Rule != RuleLogOnly || o.Decision != nil || len(o.Response.UpdatedToolOutput) != 0 {
			t.Errorf("%s: %+v", e, o)
		}
	}
}

func TestPostToolUseScrubs(t *testing.T) {
	d, _ := newDeps(t)
	token := "ghp_" + strings.Repeat("B", 36)
	resp := `{"stdout":"GITHUB_TOKEN=` + token + `\nok","stderr":"","interrupted":false,"n":3}`
	o := Handle(&hookio.Request{Event: hookio.PostToolUse, SessionID: sess, ToolName: "Bash", ToolResponse: []byte(resp)}, d)
	out := string(o.Response.UpdatedToolOutput)
	if o.Rule != RuleScrub || strings.Contains(out, token) || !strings.Contains(out, "[REDACTED:") || !strings.Contains(o.Reason, "×1") {
		t.Fatalf("not scrubbed: rule %s out %s reason %q", o.Rule, out, o.Reason)
	}
	var shape map[string]any
	if err := json.Unmarshal(o.Response.UpdatedToolOutput, &shape); err != nil || shape["interrupted"] != false || shape["n"] != float64(3) {
		t.Errorf("shape changed: %v %v", shape, err)
	}
	if o := Handle(&hookio.Request{Event: hookio.PostToolUse, SessionID: sess, ToolResponse: []byte(`{`)}, d); o.Result != "error" {
		t.Errorf("bad tool_response: %+v", o)
	}
	if o := Handle(&hookio.Request{Event: hookio.PostToolUse, SessionID: sess}, d); o.Result != "ok" || o.Rule != RuleLogOnly {
		t.Errorf("no tool_response: %+v", o)
	}
}

func TestOneAuditRecordPerInvocation(t *testing.T) {
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
	before, _ := d.Store.AuditLog().All()
	reqs := []*hookio.Request{
		{Event: hookio.PreToolUse, SessionID: sess, PromptID: "p1", ToolName: "Bash", ToolUseID: "toolu_1", ToolInput: []byte(`{"command":"ls"}`), AgentType: "athel:scout"},
		{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p2", Prompt: "approve the plan"},
		{Event: hookio.PostToolUse, SessionID: sess, ToolName: "Read", ToolInput: []byte(`{}`)},
		{Event: hookio.Stop, SessionID: sess},
	}
	for _, r := range reqs {
		if err := Record(d.Store, NewAuditRecord(r.Event, r, Handle(r, d), d.ConfigHash, 812*time.Microsecond)); err != nil {
			t.Fatal(err)
		}
		clk.Advance(time.Second)
	}
	if err := Record(d.Store, NewAuditRecord(hookio.PreToolUse, nil, Malformed(hookio.PreToolUse, errors.New("x")), "", 0)); err != nil {
		t.Fatal(err)
	}
	all, err := d.Store.AuditLog().All()
	if err != nil {
		t.Fatal(err)
	}
	recs := all[len(before):]
	if len(recs) != 5 {
		t.Fatalf("%d records, want 5", len(recs))
	}
	validateRecords(t, recs)
	checkRecords(t, recs)
	if rep, err := d.Store.AuditLog().Verify(); err != nil || !rep.OK {
		t.Errorf("chain: %+v %v", rep, err)
	}
}

func validateRecords(t *testing.T, recs []audit.Record) {
	t.Helper()
	reg := schema.NewRegistry()
	for i, r := range recs {
		b, _ := json.Marshal(r)
		var doc any
		_ = json.Unmarshal(b, &doc)
		if err := reg.Validate(schema.Audit, doc); err != nil {
			t.Errorf("record %d does not match the schema: %v\n%s", i, err, b)
		}
	}
}

func checkRecords(t *testing.T, recs []audit.Record) {
	t.Helper()
	checkToolRecord(t, recs[0])
	checkPromptRecord(t, recs[1])
	post, stop, bad := recs[2], recs[3], recs[4]
	if post.Kind != "tool_completed" || stop.Kind != "hook" || bad.Result != "error" || *bad.Decision != "deny" {
		t.Errorf("kinds: %s %s %s %v", post.Kind, stop.Kind, bad.Result, bad.Decision)
	}
	if kindOf(hookio.PostToolUseFailure) != "tool_failed" {
		t.Error("PostToolUseFailure kind")
	}
}

func checkToolRecord(t *testing.T, r audit.Record) {
	t.Helper()
	got := []string{r.Kind, *r.Change, *r.Step, *r.Task, *r.Agent, *r.Tool, *r.Decision}
	want := []string{"tool_decision", "CHG-001", "start", "T-01", "athel:scout", "Bash", "allow"}
	if strings.Join(got, ",") != strings.Join(want, ",") || r.InputSHA256 == nil || *r.DurationUS != 812 {
		t.Errorf("tool record = %+v", r)
	}
}

func checkPromptRecord(t *testing.T, r audit.Record) {
	t.Helper()
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "approve the plan") {
		t.Errorf("prompt text stored: %s", b)
	}
	if r.InputSHA256 == nil || *r.PromptID != "p2" || *r.Agent != "main" || *r.Action != "UserPromptSubmit" {
		t.Errorf("prompt record = %s", b)
	}
}

func TestGrantFromHumanPromptOnly(t *testing.T) {
	d, _ := newDeps(t)
	prompts := []*hookio.Request{
		{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p1", Prompt: "please approve CHG-001"},
		{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p2", Prompt: "/athel:approve CHG-001 then push"},
		{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p3", Prompt: "/athel:approve CHG-001", AgentID: "agent-1"},
	}
	for _, r := range prompts {
		if o := Handle(r, d); o.Rule == RuleGrant {
			t.Errorf("grant from %+v", r)
		}
	}
	if g, _ := d.Store.Grants().Valid("CHG-001", "plan_approved", sess); g != nil {
		t.Fatalf("grant created: %+v", g)
	}
	o := Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess, PromptID: "p4", Prompt: "/athel:accept CHG-001"}, d)
	g, err := d.Store.Grants().Valid("CHG-001", "done", sess)
	if o.Rule != RuleGrant || err != nil || g == nil || g.PromptID != "p4" || !strings.Contains(o.Response.AdditionalContext, g.ID) {
		t.Fatalf("literal prompt: %+v %+v %v", o, g, err)
	}
	recs, _ := d.Store.AuditLog().All()
	if last := recs[len(recs)-1]; last.Kind != audit.KindGrantCreated || *last.To != "done" || *last.PromptID != "p4" {
		t.Errorf("grant not audited: %+v", last)
	}
	if o := Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess, Prompt: "/athel:approve CHG-001"}, Deps{}); o.Rule == RuleGrant {
		t.Error("grant outside a workspace")
	}
	if err := os.WriteFile(d.Store.WS.StateDir()+"/grants.jsonl", []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if o := Handle(&hookio.Request{Event: hookio.UserPromptSubmit, SessionID: sess, Prompt: "/athel:approve CHG-001"}, d); o.Result != "error" {
		t.Errorf("corrupt book: %+v", o)
	}
}
