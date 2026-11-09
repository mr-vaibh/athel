// Package claudecode is the only code that knows Claude Code's hook contract: event names,
// input JSON fields, output JSON shape and exit codes (code.claude.com/docs/en/hooks.md,
// checked 2026-11-01). Everything else in Athel works with internal/hookio.
package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

// MaxAdditionalContext is Claude Code's cap on additionalContext ("capped at 10,000
// characters per field"); longer text is cut by Athel so the cut is visible.
const MaxAdditionalContext = 10000

// cliNames maps the `athel hook <event>` argument to the event.
var cliNames = map[string]hookio.Event{
	"session-start":         hookio.SessionStart,
	"user-prompt-submit":    hookio.UserPromptSubmit,
	"pre-tool-use":          hookio.PreToolUse,
	"post-tool-use":         hookio.PostToolUse,
	"post-tool-use-failure": hookio.PostToolUseFailure,
	"subagent-stop":         hookio.SubagentStop,
	"stop":                  hookio.Stop,
	"pre-compact":           hookio.PreCompact,
	"session-end":           hookio.SessionEnd,
}

// CLIName returns the `athel hook` argument for an event.
func CLIName(e hookio.Event) string {
	for name, ev := range cliNames {
		if ev == e {
			return name
		}
	}
	return ""
}

// EventFromCLIName parses the `athel hook <event>` argument.
func EventFromCLIName(name string) (hookio.Event, error) {
	e, ok := cliNames[name]
	if !ok {
		return "", aerr.New(aerr.StateInvalid, fmt.Sprintf("unknown hook event %q", name))
	}
	return e, nil
}

// input mirrors Claude Code's hook input. Unknown fields are ignored on purpose: Claude Code
// adds fields over time and that must not break the hook.
type input struct {
	SessionID      string          `json:"session_id"`
	PromptID       string          `json:"prompt_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	PermissionMode string          `json:"permission_mode"`
	HookEventName  string          `json:"hook_event_name"`
	AgentID        string          `json:"agent_id"`
	AgentType      string          `json:"agent_type"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	ToolUseID      string          `json:"tool_use_id"`
	Prompt         string          `json:"prompt"`
	Source         string          `json:"source"`
	Trigger        string          `json:"trigger"`
	Reason         string          `json:"reason"`
	StopHookActive bool            `json:"stop_hook_active"`
}

func malformed(event hookio.Event, why string) error {
	return aerr.New(aerr.StateInvalid, fmt.Sprintf("malformed %s hook payload: %s", event, why))
}

// ParseInput decodes a hook payload for the expected event. A payload that names a different
// event, has no session, or (for tool events) has no tool is malformed.
func ParseInput(event hookio.Event, data []byte) (*hookio.Request, error) {
	var in input
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&in); err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, fmt.Sprintf("malformed %s hook payload", event), err)
	}
	if dec.More() {
		return nil, malformed(event, "trailing data after the JSON object")
	}
	if in.HookEventName != "" && in.HookEventName != string(event) {
		return nil, malformed(event, "hook_event_name is "+in.HookEventName)
	}
	if in.SessionID == "" {
		return nil, malformed(event, "no session_id")
	}
	isTool := event == hookio.PreToolUse || event == hookio.PostToolUse || event == hookio.PostToolUseFailure
	if isTool {
		if in.ToolName == "" {
			return nil, malformed(event, "no tool_name")
		}
		if t := bytes.TrimSpace(in.ToolInput); len(t) == 0 || t[0] != '{' {
			return nil, malformed(event, "tool_input is not an object")
		}
	}
	return &hookio.Request{
		Event: event, SessionID: in.SessionID, PromptID: in.PromptID, Cwd: in.Cwd,
		TranscriptPath: in.TranscriptPath, PermissionMode: in.PermissionMode,
		AgentID: in.AgentID, AgentType: in.AgentType, ToolName: in.ToolName,
		ToolInput: []byte(in.ToolInput), ToolResponse: []byte(in.ToolResponse), ToolUseID: in.ToolUseID, Prompt: in.Prompt,
		Source: in.Source, Trigger: in.Trigger, Reason: in.Reason, StopHookActive: in.StopHookActive,
	}, nil
}

// permissionDecision maps doc 04's vocabulary to Claude Code's. Athel's "approve" means ask the
// human; Claude Code's deprecated "approve" means ALLOW, so it must never be sent. No decision
// maps to no field, which leaves the call to Claude Code's own permission rules.
func permissionDecision(d hookio.Decision) (string, error) {
	switch d {
	case "":
		return "", nil
	case hookio.Allow:
		return "allow", nil
	case hookio.Deny:
		return "deny", nil
	case hookio.Approve:
		return "ask", nil
	}
	return "", aerr.New(aerr.StateInvalid, fmt.Sprintf("no PreToolUse decision (%q)", d))
}

type specific struct {
	HookEventName            string          `json:"hookEventName"`
	PermissionDecision       string          `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string          `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string          `json:"additionalContext,omitempty"`
	UpdatedToolOutput        json.RawMessage `json:"updatedToolOutput,omitempty"`
}

type output struct {
	Decision           string    `json:"decision,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	SystemMessage      string    `json:"systemMessage,omitempty"`
	HookSpecificOutput *specific `json:"hookSpecificOutput,omitempty"`
}

// FormatOutput renders resp as the JSON Claude Code reads on stdout (with exit code 0).
func FormatOutput(event hookio.Event, resp hookio.Response) ([]byte, error) {
	out := output{SystemMessage: resp.SystemMessage}
	switch event {
	case hookio.PreToolUse:
		d, err := permissionDecision(resp.Decision)
		if err != nil {
			return nil, err
		}
		if d != "" {
			out.HookSpecificOutput = &specific{HookEventName: string(event), PermissionDecision: d, PermissionDecisionReason: resp.Reason}
		}
	case hookio.PostToolUse:
		if len(resp.UpdatedToolOutput) > 0 || resp.AdditionalContext != "" {
			out.HookSpecificOutput = &specific{HookEventName: string(event), UpdatedToolOutput: resp.UpdatedToolOutput,
				AdditionalContext: capContext(resp.AdditionalContext)}
		}
	case hookio.SessionStart, hookio.UserPromptSubmit:
		if resp.AdditionalContext != "" {
			out.HookSpecificOutput = &specific{HookEventName: string(event), AdditionalContext: capContext(resp.AdditionalContext)}
		}
	case hookio.Stop, hookio.SubagentStop:
		if resp.BlockStop {
			out.Decision, out.Reason = "block", resp.BlockReason
		}
	}
	return json.Marshal(out)
}

const truncationNote = "\n[athel: context truncated at 10,000 characters]"

func capContext(s string) string {
	if utf8.RuneCountInString(s) <= MaxAdditionalContext {
		return s
	}
	runes := []rune(s)
	keep := MaxAdditionalContext - utf8.RuneCountInString(truncationNote)
	return string(runes[:keep]) + truncationNote
}
