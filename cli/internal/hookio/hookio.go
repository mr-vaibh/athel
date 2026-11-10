// Package hookio is Athel's own model of a hook invocation, independent of Claude Code's wire
// format. The adapter in internal/adapters/claudecode translates to and from it, so a Claude
// Code change is fixed in one place (doc 02 §Cross-cutting, Compatibility).
package hookio

// Event is a hook event Athel handles (doc 02 §Runtime model, Hooks used).
type Event string

// The events wired in M0.
const (
	SessionStart       Event = "SessionStart"
	UserPromptSubmit   Event = "UserPromptSubmit"
	PreToolUse         Event = "PreToolUse"
	PostToolUse        Event = "PostToolUse"
	PostToolUseFailure Event = "PostToolUseFailure"
	SubagentStop       Event = "SubagentStop"
	Stop               Event = "Stop"
	PreCompact         Event = "PreCompact"
	SessionEnd         Event = "SessionEnd"
)

// Events lists every event Athel wires, in doc 02 order.
var Events = []Event{SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure,
	SubagentStop, Stop, PreCompact, SessionEnd}

// ScoutAgent is the agent_type Claude Code reports for Athel's scout subagent (plugin subagents
// are named <plugin>:<agent>). The scout is read-only (doc 04 injection-defence layer 4).
const ScoutAgent = "athel:scout"

// ArchitectAgent is Athel's architect subagent (doc 02 roles). It writes specs, cards and plans
// only through the athel CLI, so like the scout it never writes a file (M3 plan G13).
const ArchitectAgent = "athel:architect"

// Request is one hook invocation. Raw tool input is kept as bytes so it can be hashed without
// being interpreted; only its scrubbed, classified form is ever logged.
type Request struct {
	Event          Event
	SessionID      string
	PromptID       string
	Cwd            string
	TranscriptPath string
	PermissionMode string
	AgentID        string
	AgentType      string
	ToolName       string
	ToolInput      []byte
	ToolResponse   []byte // PostToolUse: the tool's output, scrubbed before Claude sees it
	ToolUseID      string
	Prompt         string // UserPromptSubmit only; hashed, never stored
	Source         string // SessionStart: startup | resume | clear | compact | fork
	Trigger        string // PreCompact: manual | auto
	Reason         string // SessionEnd
	StopHookActive bool   // Stop / SubagentStop: already continuing because of a stop hook
}

// Decision is a PreToolUse policy decision in doc 04's vocabulary.
type Decision string

// Decisions. Approve means "ask the human" (doc 04), not "allow".
const (
	Allow   Decision = "allow"
	Deny    Decision = "deny"
	Approve Decision = "approve"
)

// Response is what Athel wants Claude Code to do.
type Response struct {
	Decision          Decision // PreToolUse only; empty leaves the call to Claude Code's own rules
	Reason            string   // shown to the model (deny) or the human (approve)
	AdditionalContext string   // SessionStart / UserPromptSubmit / PostToolUse
	SystemMessage     string   // shown to the user
	BlockStop         bool     // Stop / SubagentStop: keep working
	BlockReason       string
	UpdatedToolOutput []byte // PostToolUse: replaces the tool output Claude sees (same JSON shape)
}
