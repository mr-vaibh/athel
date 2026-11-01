package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestAuditVerifyAndTail(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	h.runJSON("state", "transition", "CHG-001", "--to", "done", "--by", "athel", "--reason", "skip")
	rep := h.mustOK("audit", "verify")
	if rep["ok"] != true || rep["records"] != 2.0 {
		t.Errorf("verify = %v", rep)
	}
	code, env := h.runJSON("audit", "tail", "-n", "1")
	if recs := env["data"].([]any); code != 0 || len(recs) != 1 || recs[0].(map[string]any)["kind"] != "illegal_transition" {
		t.Errorf("tail = %v", env)
	}
	if code, _ := h.runJSON("audit", "tail", "-n", "0"); code != aerr.ExitUsage {
		t.Errorf("tail -n 0: exit %d", code)
	}
	path := filepath.Join(h.root, ".athel", "audit", "2026-11-01.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(b, []byte(`"reason":"r"`), []byte(`"reason":"x"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	code, env = h.runJSON("audit", "verify")
	if code != aerr.ExitIntegrity || errCode(env) != string(aerr.IntegrityAudit) {
		t.Errorf("tampered: exit %d env %v", code, env)
	}
}

func TestAuditCommandErrors(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	path := filepath.Join(h.root, ".athel", "audit", "2026-11-01.jsonl")
	if code, _ := h.runJSON("audit", "verify", "x"); code != aerr.ExitUsage {
		t.Errorf("extra arg: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("audit", "verify"); code != aerr.ExitState {
		t.Errorf("outside a workspace: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("audit", "tail"); code != aerr.ExitState {
		t.Errorf("tail outside a workspace: exit %d", code)
	}
	if err := os.WriteFile(path, []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("audit", "tail"); code != aerr.ExitIntegrity {
		t.Errorf("tail of corrupt log: exit %d", code)
	}
}
