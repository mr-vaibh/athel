package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func (h *harness) decide(tool string, input map[string]any, cwd string) map[string]any {
	h.t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		h.t.Fatal(err)
	}
	return h.mustOK("policy", "decide", "--tool", tool, "--input", string(raw), "--cwd", cwd)
}

func TestPolicyDecideGuardMode(t *testing.T) {
	h := newHarness(t)
	h.userHome = t.TempDir()
	h.mustOK("init")
	edit := map[string]any{"file_path": filepath.Join(h.root, "a.go"), "old_string": "a", "new_string": "b"}
	if v := h.decide("Edit", edit, h.root); v["decision"] != "none" || v["mode"] != "guard" {
		t.Errorf("guard edit = %v", v)
	}
	if v := h.decide("Bash", map[string]any{"command": "kubectl delete pod x"}, h.root); v["decision"] != "deny" || v["code"] != string(aerr.PolicyProdWrite) {
		t.Errorf("unknown cluster write = %v", v)
	}
	h.env["ATHEL_HEADLESS"] = "1"
	if v := h.decide("Bash", map[string]any{"command": "cat $'x'"}, h.root); v["decision"] != "none" {
		t.Errorf("guard read = %v", v)
	}
	outside := newHarness(t)
	outside.userHome = t.TempDir()
	if v := outside.decide("Read", map[string]any{"file_path": "/x/.env"}, outside.root); v["decision"] != "deny" || v["mode"] != "guard" {
		t.Errorf("outside a workspace = %v", v)
	}
}

func TestPolicyDecideChangeMode(t *testing.T) {
	h := setup(t)
	h.userHome = t.TempDir()
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "start")
	tree := filepath.Join(h.root, "repos", "r@CHG-001")
	edit := map[string]any{"file_path": filepath.Join(tree, "src", "a.go"), "old_string": "a", "new_string": "b"}
	if v := h.decide("Edit", edit, tree); v["decision"] != "deny" || v["code"] != string(aerr.PolicyScope) || v["mode"] != "change" {
		t.Errorf("edit without a plan = %v", v)
	}
	m3Spec(t, h)
	m3Plan(t, h)
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
	if v := h.decide("Edit", edit, tree); v["decision"] != "allow" || v["rule"] != "10" {
		t.Errorf("planned edit = %v", v)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-B"
	if v := h.decide("Edit", edit, tree); v["decision"] != "none" || v["mode"] != "guard" {
		t.Errorf("another session = %v", v)
	}
}

func TestPolicyDecideErrors(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	if code, env := h.runJSON("policy", "decide", "--tool", "Read", "--input", "{"); code != aerr.ExitState || errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("bad json: %d %v", code, env)
	}
	for _, args := range [][]string{{"x"}, {"--tool", "Read"}, {"--input", "{}"}} {
		if code, _ := h.runJSON(append([]string{"policy", "decide"}, args...)...); code != aerr.ExitUsage {
			t.Errorf("usage %v: exit %d", args, code)
		}
	}
	write := []string{"policy", "decide", "--tool", "Write", "--input", `{"file_path":"a","content":"x"}`}
	denied := func(what string) {
		t.Helper()
		code, env := h.runJSON(write...)
		data, _ := env["data"].(map[string]any)
		if code != 0 || data["decision"] != "deny" || data["code"] != string(aerr.PolicyConfigInvalid) || data["rule"] != "0" {
			t.Errorf("%s: %d %v", what, code, env)
		}
	}
	denied("no home")
	h.userHome = t.TempDir()
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "tools.yaml"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	denied("invalid tools.yaml")
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "athel.yaml"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	denied("invalid athel.yaml")
}
