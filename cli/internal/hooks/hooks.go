// Package hooks decides what Athel does on each Claude Code hook event in M0 and records one
// line per invocation in .athel/audit/hook-events.jsonl.
//
// M0 STUB WARNING: PreToolUse allows every well-formed tool call. It enforces NO policy. The
// real policy engine (tiers, environments, plan scope, forbidden patterns) is milestone M1.
package hooks

import (
	"fmt"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	"github.com/mr-vaibh/athel/cli/internal/eventlog"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// StubReason is returned with every M0 PreToolUse allow so nobody mistakes it for a policy.
const StubReason = "ATHEL M0 STUB: no policy enforcement (real policy engine is M1)"

// Rule names recorded in hook events.
const (
	RuleStub      = "m0-stub"
	RuleMalformed = "m0-malformed-payload"
	RuleLogOnly   = "m0-log-only"
)

// StopBlockLimit is doc 02's limit: after 3 blocked stops the step moves to blocked.
const StopBlockLimit = 3

// Deps is everything a handler may touch. Store is nil outside an Athel workspace; then hooks
// still answer Claude Code but have nothing to read or record.
type Deps struct {
	Store         *state.Store
	Clock         clock.Clock
	ConfigHash    string
	ConfigErr     error
	EnvFile       string // CLAUDE_ENV_FILE, SessionStart only
	MinClaudeCode string
	ClaudeVersion func() (string, error)
	AppendEnv     func(path, line string) error
}

// Outcome is a handler's answer plus what the audit line should say about it.
type Outcome struct {
	Response hookio.Response
	Change   *state.Change
	Decision *hookio.Decision
	Rule     string
	Result   string // ok | error
	Notes    []string
}

// Handle runs the M0 behaviour for one parsed request.
func Handle(req *hookio.Request, d Deps) Outcome {
	switch req.Event {
	case hookio.PreToolUse:
		return preToolUse(req, d)
	case hookio.SessionStart:
		return sessionStart(req, d)
	case hookio.UserPromptSubmit:
		return userPromptSubmit(req, d)
	case hookio.PreCompact:
		return preCompact(req, d)
	case hookio.SessionEnd:
		return sessionEnd(req, d)
	case hookio.Stop, hookio.SubagentStop:
		return stop(req, d)
	}
	return Outcome{Rule: RuleLogOnly, Result: "ok", Change: activeChange(d, req.SessionID)}
}

// Malformed is the answer to a payload the adapter could not parse. PreToolUse denies (doc 04
// decision rule 1, decision Q9); other events answer nothing.
func Malformed(event hookio.Event, err error) Outcome {
	o := Outcome{Rule: RuleMalformed, Result: "error"}
	if event == hookio.PreToolUse {
		deny := hookio.Deny
		o.Decision = &deny
		o.Response = hookio.Response{Decision: deny, Reason: "Athel: malformed hook payload, denied: " + err.Error()}
	}
	return o
}

func preToolUse(req *hookio.Request, d Deps) Outcome {
	allow := hookio.Allow
	o := Outcome{Decision: &allow, Rule: RuleStub, Result: "ok",
		Response: hookio.Response{Decision: allow, Reason: StubReason}}
	o.Change = activeChange(d, req.SessionID)
	if o.Change != nil {
		if _, err := d.Store.RefreshLock(o.Change.ID, req.SessionID, false); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "heartbeat: "+err.Error())
		}
	}
	return o
}

func userPromptSubmit(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok", Change: activeChange(d, req.SessionID)}
	if c := o.Change; c != nil {
		step := "no step running"
		if c.Step != nil && c.Step.Status == state.StepRunning {
			step = "step " + c.Step.Name
		}
		o.Response.AdditionalContext = fmt.Sprintf("Athel: active change %s (%s, %s).", c.ID, c.State, step)
	}
	return o
}

func preCompact(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok", Change: activeChange(d, req.SessionID)}
	c := o.Change
	if c == nil {
		return o
	}
	name := "precompact"
	if c.Step != nil && c.Step.Status == state.StepRunning {
		name = c.Step.Name + ".precompact"
	}
	note := "context compaction (" + orUnknown(req.Trigger) + ")"
	updated, err := d.Store.AddCheckpoint(c.ID, req.SessionID, name, &note, state.TriggerPreCompact)
	if err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "checkpoint: "+err.Error())
		return o
	}
	o.Change = updated
	return o
}

func sessionEnd(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok"}
	if d.Store == nil {
		return o
	}
	released, err := d.Store.ReleaseAllMine(req.SessionID)
	if err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "release: "+err.Error())
	}
	if len(released) > 0 {
		o.Response.SystemMessage = "Athel released locks: " + strings.Join(released, ", ")
	}
	return o
}

// stop logs only in M0. ShouldBlockStop carries the loop protection M1+ gates will use.
func stop(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok", Change: activeChange(d, req.SessionID)}
	o.Response.BlockStop = ShouldBlockStop(req, false, 0)
	return o
}

// ShouldBlockStop decides whether a Stop/SubagentStop hook may block. It never blocks while
// Claude Code is already continuing because of a stop hook (stop_hook_active), and never more
// than StopBlockLimit times in a row (Athel's own counter in change.yaml), so a failing gate can
// never loop forever (doc 02 §Runtime model).
func ShouldBlockStop(req *hookio.Request, gatesRed bool, consecutiveBlocks int) bool {
	return gatesRed && !req.StopHookActive && consecutiveBlocks < StopBlockLimit
}

// activeChange is the change this session holds the lock on, or nil.
func activeChange(d Deps, session string) *state.Change {
	if d.Store == nil || session == "" {
		return nil
	}
	changes, err := d.Store.List()
	if err != nil {
		return nil
	}
	for _, c := range changes {
		if c.Lock != nil && c.Lock.Session == session {
			return c
		}
	}
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// HookEvent is one line of audit/hook-events.jsonl (schema athel/hook-event@1). Field names
// follow the doc 04 audit record; tool input is stored only as a hash (no scrubber in M0).
type HookEvent struct {
	Schema      string           `json:"schema"`
	Seq         int              `json:"seq"`
	TS          string           `json:"ts"`
	Event       hookio.Event     `json:"event"`
	Session     *string          `json:"session"`
	PromptID    *string          `json:"prompt_id"`
	Agent       *string          `json:"agent"`
	Change      *string          `json:"change"`
	Step        *string          `json:"step"`
	Task        *string          `json:"task"`
	Tool        *string          `json:"tool"`
	ToolUseID   *string          `json:"tool_use_id"`
	InputSHA256 *string          `json:"input_sha256"`
	Tier        *string          `json:"tier"`
	Env         *string          `json:"env"`
	Decision    *hookio.Decision `json:"decision"`
	Rule        *string          `json:"rule"`
	PolicyHash  *string          `json:"policy_hash"`
	Result      string           `json:"result"`
	PrevHash    *string          `json:"prev_hash"`
	DurationUS  int64            `json:"duration_us"`
}

// Stamp sets the sequence number and timestamp (eventlog.Record).
func (e *HookEvent) Stamp(seq int, ts string) { e.Seq, e.TS = seq, ts }

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NewHookEvent builds the audit line for one invocation. Event is the parsed event or, for a
// malformed payload, the event named on the command line.
func NewHookEvent(event hookio.Event, req *hookio.Request, o Outcome, policyHash string, took time.Duration) *HookEvent {
	e := &HookEvent{Schema: "athel/hook-event@1", Event: event, Decision: o.Decision, Rule: opt(o.Rule),
		PolicyHash: opt(policyHash), Result: o.Result, DurationUS: took.Microseconds()}
	if req != nil {
		e.Session, e.PromptID, e.Tool, e.ToolUseID = opt(req.SessionID), opt(req.PromptID), opt(req.ToolName), opt(req.ToolUseID)
		e.Agent = opt(req.AgentType)
		if e.Agent == nil {
			e.Agent = opt("main")
		}
		switch {
		case len(req.ToolInput) > 0:
			e.InputSHA256 = opt(digest.Bytes(req.ToolInput))
		case req.Prompt != "":
			e.InputSHA256 = opt(digest.Bytes([]byte(req.Prompt)))
		}
	}
	if c := o.Change; c != nil {
		e.Change = opt(c.ID)
		if c.Step != nil {
			e.Step, e.Task = opt(c.Step.Name), c.Step.Task
		}
	}
	return e
}

// Record appends the hook event to the workspace's hook log.
func Record(store *state.Store, e *HookEvent) error {
	l := &eventlog.Log{Path: HookEventsFile(store.WS), Clock: store.Clock, LockTimeout: store.LockTimeout}
	return l.Append(e)
}

// HookEventsFile is .athel/audit/hook-events.jsonl.
func HookEventsFile(ws state.Workspace) string { return ws.AuditDir() + "/hook-events.jsonl" }
