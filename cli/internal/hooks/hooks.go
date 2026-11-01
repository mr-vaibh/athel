// Package hooks decides what Athel does on each Claude Code hook event and records one audit
// record per invocation in the hash-chained audit log (.athel/audit/YYYY-MM-DD.jsonl).
//
// M0 STUB WARNING: PreToolUse allows every well-formed tool call. It enforces NO policy. The
// real policy engine (tiers, environments, plan scope, forbidden patterns) is milestone M1.
package hooks

import (
	"fmt"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
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

// kindOf maps a hook event to its audit record kind.
func kindOf(event hookio.Event) string {
	switch event {
	case hookio.PreToolUse:
		return audit.KindToolDecision
	case hookio.PostToolUse:
		return audit.KindToolCompleted
	case hookio.PostToolUseFailure:
		return audit.KindToolFailed
	}
	return audit.KindHook
}

// NewAuditRecord builds the audit record for one hook invocation. Event is the parsed event
// or, for a malformed payload, the event named on the command line. Raw tool input and prompt
// text are never stored, only their SHA-256.
func NewAuditRecord(event hookio.Event, req *hookio.Request, o Outcome, policyHash string, took time.Duration) audit.Record {
	us := took.Microseconds()
	r := audit.Record{Kind: kindOf(event), Rule: audit.Str(o.Rule), PolicyHash: audit.Str(policyHash),
		Result: o.Result, DurationUS: &us}
	if r.Kind == audit.KindHook {
		r.Action = audit.Str(string(event))
	}
	if o.Decision != nil {
		r.Decision = audit.Str(string(*o.Decision))
	}
	if req != nil {
		r.Session, r.PromptID = audit.Str(req.SessionID), audit.Str(req.PromptID)
		r.Tool, r.ToolUseID = audit.Str(req.ToolName), audit.Str(req.ToolUseID)
		r.Agent = audit.Str(req.AgentType)
		if r.Agent == nil {
			r.Agent = audit.Str("main")
		}
		switch {
		case len(req.ToolInput) > 0:
			r.InputSHA256 = audit.Str(digest.Bytes(req.ToolInput))
		case req.Prompt != "":
			r.InputSHA256 = audit.Str(digest.Bytes([]byte(req.Prompt)))
		}
	}
	if c := o.Change; c != nil {
		r.Change = audit.Str(c.ID)
		if c.Step != nil {
			r.Step, r.Task = audit.Str(c.Step.Name), c.Step.Task
		}
	}
	return r
}

// Record appends the hook's audit record to the workspace's audit log.
func Record(store *state.Store, r audit.Record) error {
	_, err := store.AuditLog().Append(r)
	return err
}
