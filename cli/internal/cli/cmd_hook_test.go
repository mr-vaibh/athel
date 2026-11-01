package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func payload(t *testing.T, h *harness, fields map[string]any) {
	t.Helper()
	fields["cwd"] = h.root
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	h.stdin = string(b)
}

// humanPrompt sends the developer's prompt through the UserPromptSubmit hook, as Claude Code
// does when the human types it; it is the only way a test can create a human-origin grant.
func (h *harness) humanPrompt(prompt string) {
	h.t.Helper()
	payload(h.t, h, map[string]any{"session_id": h.env["ATHEL_SESSION_ID"], "prompt_id": "prompt-" + prompt, "prompt": prompt})
	hookOut(h.t, h, "user-prompt-submit")
}

func hookOut(t *testing.T, h *harness, event string) map[string]any {
	t.Helper()
	code, out, _ := h.run("hook", event)
	if code != 0 {
		t.Fatalf("hook %s exit %d", event, code)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("hook output is not JSON: %q", out)
	}
	return m
}

// hookLines returns the audit records written by hooks, as JSON lines.
func hookLines(t *testing.T, h *harness) []string {
	t.Helper()
	l := &audit.Log{Dir: filepath.Join(h.root, ".athel", "audit"), Clock: h.clock, LockTimeout: time.Second}
	recs, err := l.All()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recs {
		if r.Kind == audit.KindToolDecision || r.Kind == audit.KindToolCompleted || r.Kind == audit.KindToolFailed || r.Kind == audit.KindHook {
			b, _ := json.Marshal(r)
			out = append(out, string(b))
		}
	}
	return out
}

func decisionOf(out map[string]any) any {
	if hso, ok := out["hookSpecificOutput"].(map[string]any); ok {
		return hso["permissionDecision"]
	}
	return nil
}

func TestPreToolUseDecisions(t *testing.T) {
	h := setup(t)
	h.userHome = t.TempDir()
	bash := func(cmd string) map[string]any {
		payload(t, h, map[string]any{"session_id": "sess-A", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": cmd}})
		return hookOut(t, h, "pre-tool-use")
	}
	if out := bash("ls"); len(out) != 0 {
		t.Errorf("guard-mode read must leave the call to Claude Code: %v", out)
	}
	out := bash("kubectl delete pod x")
	if decisionOf(out) != "deny" || !strings.Contains(out["hookSpecificOutput"].(map[string]any)["permissionDecisionReason"].(string), string(aerr.PolicyProdWrite)) {
		t.Errorf("prod write = %v", out)
	}
	h.mustOK("lock", "acquire", "CHG-001")
	if d := decisionOf(bash("cat /usr/share/dict/words")); d != "ask" {
		t.Errorf("change-mode read outside the workspace = %v", d)
	}
	if d := decisionOf(bash("echo x > " + filepath.Join(h.root, "README.md"))); d != "deny" {
		t.Errorf("change-mode write without a plan = %v", d)
	}
	h.stdin = "not json"
	if decisionOf(hookOut(t, h, "pre-tool-use")) != "deny" {
		t.Error("malformed payload was not denied")
	}
	if h.stdin = strings.Repeat(" ", maxHookInput+1); decisionOf(hookOut(t, h, "pre-tool-use")) != "deny" {
		t.Error("oversized payload was not denied")
	}
	lines := hookLines(t, h)
	want := []string{`"decision":"none"`, `"decision":"deny"`, `"decision":"approve"`, `"decision":"deny"`, `"rule":"1"`, `"rule":"1"`}
	if len(lines) != len(want) {
		t.Fatalf("hook events = %v", lines)
	}
	for i, w := range want {
		if !strings.Contains(lines[i], w) {
			t.Errorf("record %d lacks %s: %s", i, w, lines[i])
		}
	}
	if !strings.Contains(lines[1], `"action":"kubectl delete pod x"`) || !strings.Contains(lines[1], `"env":"prod"`) || !strings.Contains(lines[1], `"tier":"W2"`) && !strings.Contains(lines[1], `"tier":"W3"`) {
		t.Errorf("decision record = %s", lines[1])
	}
}

func TestPostToolUseScrubsThroughCLI(t *testing.T) {
	h := setup(t)
	token := "ghp_" + strings.Repeat("C", 36)
	payload(t, h, map[string]any{"session_id": "sess-A", "tool_name": "Bash", "tool_input": map[string]any{"command": "env"},
		"tool_response": map[string]any{"stdout": "GITHUB_TOKEN=" + token, "stderr": "", "interrupted": false}})
	code, raw, _ := h.run("hook", "post-tool-use")
	if code != 0 || strings.Contains(raw, token) || !strings.Contains(raw, `"updatedToolOutput":{`) || !strings.Contains(raw, "[REDACTED:") {
		t.Errorf("exit %d output %s", code, raw)
	}
	lines := hookLines(t, h)
	if len(lines) != 1 || strings.Contains(lines[0], token) || !strings.Contains(lines[0], `"rule":"scrub"`) {
		t.Errorf("audit = %v", lines)
	}
}

func TestHookSessionLifecycleThroughCLI(t *testing.T) {
	h := setup(t)
	envFile := filepath.Join(t.TempDir(), "claude-env")
	h.env["CLAUDE_ENV_FILE"] = envFile
	payload(t, h, map[string]any{"session_id": "sess-A", "source": "startup"})
	out := hookOut(t, h, "session-start")
	ctx := out["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(ctx, `CHG-001 "Phone update": draft`) {
		t.Errorf("context = %q", ctx)
	}
	if b, err := os.ReadFile(envFile); err != nil || string(b) != "export ATHEL_SESSION_ID=sess-A\n" {
		t.Errorf("env file = %q %v", b, err)
	}
	h.mustOK("lock", "acquire", "CHG-001")
	payload(t, h, map[string]any{"session_id": "sess-A", "prompt_id": "p-1", "prompt": "hello"})
	if ctx := hookOut(t, h, "user-prompt-submit")["hookSpecificOutput"].(map[string]any)["additionalContext"]; ctx != "Athel: active change CHG-001 (draft, no step running)." {
		t.Errorf("prompt context = %v", ctx)
	}
	payload(t, h, map[string]any{"session_id": "sess-A", "trigger": "manual"})
	hookOut(t, h, "pre-compact")
	if cp := h.mustOK("checkpoint", "show", "CHG-001")["last"].(map[string]any); cp["name"] != "precompact" || cp["trigger"] != "precompact" {
		t.Errorf("precompact checkpoint = %v", cp)
	}
	payload(t, h, map[string]any{"session_id": "sess-A", "stop_hook_active": true})
	if out := hookOut(t, h, "stop"); len(out) != 0 {
		t.Errorf("stop output = %v", out)
	}
	payload(t, h, map[string]any{"session_id": "sess-A", "reason": "logout"})
	if out := hookOut(t, h, "session-end"); out["systemMessage"] != "Athel released locks: CHG-001" {
		t.Errorf("session end = %v", out)
	}
	if lines := hookLines(t, h); len(lines) != 5 {
		t.Errorf("%d hook events, want 5", len(lines))
	}
}

func TestHookOutsideWorkspaceAndBadConfig(t *testing.T) {
	h := newHarness(t)
	payload(t, h, map[string]any{"session_id": "s", "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
	if out := hookOut(t, h, "pre-tool-use"); len(out) != 0 {
		t.Errorf("read outside a workspace: %v", out)
	}
	payload(t, h, map[string]any{"session_id": "s", "tool_name": "Write", "tool_input": map[string]any{"file_path": "/x/a", "content": "x"}})
	if d := decisionOf(hookOut(t, h, "pre-tool-use")); d != "deny" {
		t.Errorf("write with an unreadable home folder must fail closed: %v", d)
	}
	h.mustOK("init")
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload(t, h, map[string]any{"session_id": "s"})
	ctx := hookOut(t, h, "session-start")["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.Contains(ctx, "CONFIGURATION INVALID") {
		t.Errorf("context = %q", ctx)
	}
}

func TestHookAuditFailureDoesNotChangeAnswer(t *testing.T) {
	h := setup(t)
	audit := filepath.Join(h.root, ".athel", "audit")
	if err := os.RemoveAll(audit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audit, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h.userHome = t.TempDir()
	payload(t, h, map[string]any{"session_id": "s", "tool_name": "Bash", "tool_input": map[string]any{"command": "kubectl delete pod x"}})
	code, out, errOut := h.run("hook", "pre-tool-use")
	if code != 0 || !strings.Contains(out, `"deny"`) || !strings.Contains(errOut, "audit write failed") {
		t.Errorf("exit %d out %q err %q", code, out, errOut)
	}
}

func TestHookUsage(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{{"hook"}, {"hook", "PreToolUse"}, {"hook", "a", "b"}} {
		if code, _, _ := h.run(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	h.stdin = `{"session_id":"s"}`
	if code, out, _ := h.run("--json", "hook", "stop"); code != 0 || strings.TrimSpace(out) != "{}" {
		t.Errorf("--json must not wrap hook output: %d %q", code, out)
	}
}

func TestSubagentCannotUseGrant(t *testing.T) {
	h := setup(t)
	h.userHome = t.TempDir()
	h.mustOK("lock", "acquire", "CHG-001")
	h.humanPrompt("/athel:accept CHG-001")
	call := func(agentID string) any {
		fields := map[string]any{"session_id": "sess-A", "tool_name": "Bash",
			"tool_input": map[string]any{"command": "athel state transition CHG-001 --to done --by human --reason accepted"}}
		if agentID != "" {
			fields["agent_id"], fields["agent_type"] = agentID, "athel:developer"
		}
		payload(t, h, fields)
		return decisionOf(hookOut(t, h, "pre-tool-use"))
	}
	if d := call("agent-1"); d != "deny" {
		t.Errorf("subagent with the human's grant = %v", d)
	}
	if d := call(""); d != "allow" {
		t.Errorf("main agent with the human's grant = %v", d)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-B"
	payload(t, h, map[string]any{"session_id": "sess-B", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "athel state transition CHG-001 --to done --by human --reason accepted"}})
	if d := decisionOf(hookOut(t, h, "pre-tool-use")); d != "deny" {
		t.Errorf("another session = %v", d)
	}
}
