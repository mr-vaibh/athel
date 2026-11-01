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

func TestHookPreToolUseThroughCLI(t *testing.T) {
	h := setup(t)
	payload(t, h, map[string]any{"session_id": "sess-A", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
	out := hookOut(t, h, "pre-tool-use")
	hso := out["hookSpecificOutput"].(map[string]any)
	if hso["permissionDecision"] != "allow" || !strings.Contains(hso["permissionDecisionReason"].(string), "M0 STUB") {
		t.Errorf("output = %v", out)
	}
	h.stdin = "not json"
	out = hookOut(t, h, "pre-tool-use")
	if out["hookSpecificOutput"].(map[string]any)["permissionDecision"] != "deny" {
		t.Errorf("malformed = %v", out)
	}
	if h.stdin = strings.Repeat(" ", maxHookInput+1); hookOut(t, h, "pre-tool-use")["hookSpecificOutput"].(map[string]any)["permissionDecision"] != "deny" {
		t.Error("oversized payload was not denied")
	}
	lines := hookLines(t, h)
	if len(lines) != 3 || !strings.Contains(lines[0], `"decision":"allow"`) || !strings.Contains(lines[1], `"decision":"deny"`) ||
		!strings.Contains(lines[2], `"rule":"m0-malformed-payload"`) {
		t.Errorf("hook events = %v", lines)
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
	payload(t, h, map[string]any{"session_id": "s", "tool_name": "Bash", "tool_input": map[string]any{}})
	if d := hookOut(t, h, "pre-tool-use")["hookSpecificOutput"].(map[string]any)["permissionDecision"]; d != "allow" {
		t.Errorf("no workspace: %v", d)
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
	payload(t, h, map[string]any{"session_id": "s", "tool_name": "Bash", "tool_input": map[string]any{}})
	code, out, errOut := h.run("hook", "pre-tool-use")
	if code != 0 || !strings.Contains(out, `"allow"`) || !strings.Contains(errOut, "audit write failed") {
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
