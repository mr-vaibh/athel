// Package hooks decides what Athel does on each Claude Code hook event and records one audit
// record per invocation in the hash-chained audit log (.athel/audit/YYYY-MM-DD.jsonl).
// PreToolUse asks the policy engine (doc 04, ADR 015); PostToolUse scrubs secrets from the
// tool output before Claude sees it (FR-SAFE-008).
package hooks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	"github.com/mr-vaibh/athel/cli/internal/engine"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/scrub"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Rule names recorded in hook events that the policy engine does not decide.
const (
	RuleMalformed = "1"
	RuleLogOnly   = "log-only"
	RuleScrub     = "scrub"
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
	// Decide is the policy decision for a PreToolUse call. It never fails: a policy or state
	// problem must come back as rule 0 (deny writes), never as "no decision".
	Decide func(req *hookio.Request) engine.Result
	Scrub  *scrub.Scrubber
}

// Outcome is a handler's answer plus what the audit line should say about it.
type Outcome struct {
	Response hookio.Response
	Change   *state.Change
	Decision *hookio.Decision
	Rule     string
	Result   string // ok | error
	Notes    []string
	Policy   *engine.Result // PreToolUse: the engine's full decision
	Action   string         // scrubbed description of what was decided or scrubbed
	Reason   string         // scrubbed reason recorded in the audit log
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
	case hookio.PostToolUse:
		return postToolUse(req, d)
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
		o.Response = hookio.Response{Decision: deny, Reason: "Athel: malformed hook payload, denied: " + err.Error() + " [E-POLICY-MALFORMED, rule 1]"}
	}
	return o
}

// hookDecisions maps the engine's decisions to the hook's; none has no entry (no output).
var hookDecisions = map[engine.Decision]hookio.Decision{
	engine.Allow: hookio.Allow, engine.Approve: hookio.Approve, engine.Deny: hookio.Deny,
}

func preToolUse(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Result: "ok", Change: activeChange(d, req.SessionID)}
	if o.Change != nil {
		if _, err := d.Store.RefreshLock(o.Change.ID, req.SessionID, false); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "heartbeat: "+err.Error())
		}
	}
	res := d.Decide(req)
	o.Policy, o.Rule = &res, res.Rule
	texts := make([]string, 0, len(res.Actions))
	for _, a := range res.Actions {
		texts = append(texts, a.Action.Text)
	}
	o.Action, _ = d.Scrub.String(strings.Join(texts, " ; "))
	if dec, ok := hookDecisions[res.Decision]; ok {
		o.Decision = &dec
		o.Response = hookio.Response{Decision: dec, Reason: res.Reason}
		o.Reason, _ = d.Scrub.String(res.Reason)
	}
	return o
}

// postToolUse scrubs secrets from the tool output before Claude sees it (FR-SAFE-008). Only
// string leaves change, so the output keeps the shape Claude Code expects.
func postToolUse(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok", Change: activeChange(d, req.SessionID)}
	if len(req.ToolResponse) == 0 {
		return o
	}
	var v any
	if err := json.Unmarshal(req.ToolResponse, &v); err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "tool_response is not JSON: "+err.Error())
		return o
	}
	clean, found := d.Scrub.Value(v)
	if len(found) == 0 {
		return o
	}
	out, _ := json.Marshal(clean) // values decoded from JSON always encode
	kinds := make([]string, 0, len(found))
	for _, f := range found {
		kinds = append(kinds, fmt.Sprintf("%s×%d", f.Kind, f.Count))
	}
	o.Rule, o.Response.UpdatedToolOutput = RuleScrub, out
	o.Reason = "scrubbed from the tool output: " + strings.Join(kinds, ", ")
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
	if p := o.Policy; p != nil {
		r.Decision, r.Tier, r.Env, r.Code = audit.Str(string(p.Decision)), audit.Str(string(p.Tier)), audit.Str(p.Env), audit.Str(string(p.Code))
	}
	if o.Action != "" {
		r.Action = audit.Str(o.Action)
	}
	r.Reason = audit.Str(o.Reason)
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
