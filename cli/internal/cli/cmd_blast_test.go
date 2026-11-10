package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const blastDoc = `schema: athel/blast@1
ask: ASK-001
mode: adhoc
proposal: Rename phone to phone_number in the users API
services_changed: [users]
direct:
  - {item: "PUT /v1/users/{id}", kind: interface, change: breaking, service: users, evidence: [{kind: code, ref: users/api.go:12}]}
consumers:
  - {consumer: notifications, of: "PUT /v1/users/{id}", env: prod, in_change_set: false, traffic: {rate: 3, unit: calls_per_min, window: 7d, sampling: 1}, evidence: [{kind: trace, ref: tempo}], facts: []}
data: []
infra: []
people: []
runtime:
  - {service: users, env: prod, calls_per_sec: 5, error_rate: 0, alerts: [], evidence: [{kind: metric, ref: grafana}]}
flags: {touches_auth_payments_pii: false, security_review_task: false, retry_timeout_or_idempotency_change: false}
not_affected: []
gaps: []
risk: LOW
rules_fired: []
`

func TestBlastCommands(t *testing.T) {
	h := setup(t)
	h.mustOK("ask", "new", "--kind", "blast", "--question", "Rename phone")
	file := writeTemp(t, "blast.yaml", blastDoc)
	s := h.mustOK("blast", "score", file)
	if s["risk"] != "CRITICAL" || s["rules_fired"].([]any)[0].(map[string]any)["rule"] != "breaking-consumer-outside-set" {
		t.Fatalf("score = %v", s)
	}
	saved := h.mustOK("blast", "save", "--ask", "ASK-001", "--summary-text", "Breaking for notifications.", file)
	md, err := os.ReadFile(saved["path"].(string))
	if err != nil || !strings.Contains(string(md), "risk CRITICAL") || strings.Contains(string(md), "risk: LOW") {
		t.Fatalf("blast-radius.md (the model's LOW must be replaced) = %s %v", md, err)
	}
	h.stdin = strings.Replace(blastDoc, "ask: ASK-001", "change: CHG-001", 1)
	if v := h.mustOK("blast", "save", "--change", "CHG-001", "-"); !strings.HasSuffix(v["path"].(string), filepath.Join("CHG-001", "blast-radius.md")) {
		t.Errorf("change save = %v", v)
	}
}

func TestBlastCommandErrors(t *testing.T) {
	h := setup(t)
	file := writeTemp(t, "blast.yaml", blastDoc)
	tests := []struct {
		args []string
		code int
	}{
		{[]string{"blast", "score"}, aerr.ExitUsage},
		{[]string{"blast", "save", file}, aerr.ExitUsage},
		{[]string{"blast", "save", "--ask", "ASK-001", "--change", "CHG-001", file}, aerr.ExitUsage},
		{[]string{"blast", "score", filepath.Join(t.TempDir(), "none")}, aerr.ExitState},
		{[]string{"blast", "score", writeTemp(t, "bad.yaml", "schema: x\n")}, aerr.ExitState},
		{[]string{"blast", "save", "--ask", "ASK-009", file}, aerr.ExitState},
		{[]string{"blast", "save", "--change", "CHG-001", file}, aerr.ExitState},
	}
	for _, tt := range tests {
		if code, env := h.runJSON(tt.args...); code != tt.code {
			t.Errorf("%v: exit %d, want %d (%v)", tt.args, code, tt.code, env)
		}
	}
	if code, env := h.runJSON("blast", "save", "--ask", "ASK-001", writeTemp(t, "bad.yaml", "schema: x\n")); code != aerr.ExitState ||
		!strings.Contains(asJSON(t, env), "run athel blast template for the format") {
		t.Errorf("save of an invalid report points to the template: exit %d %v", code, env)
	}
	h.mustOK("lock", "acquire", "CHG-001")
	h.env["ATHEL_SESSION_ID"] = "sess-B"
	h.stdin = strings.Replace(blastDoc, "ask: ASK-001", "change: CHG-001", 1)
	if code, _ := h.runJSON("blast", "save", "--change", "CHG-001", "-"); code != aerr.ExitState {
		t.Errorf("save while another session holds the change: exit %d", code)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	outside := newHarness(t)
	if v := outside.mustOK("blast", "score", file); v["risk"] != "CRITICAL" {
		t.Errorf("score outside a workspace = %v", v)
	}
	if code, _ := outside.runJSON("blast", "save", "--ask", "ASK-001", file); code != aerr.ExitState {
		t.Errorf("save outside a workspace: exit %d", code)
	}
	noHome := newHarness(t)
	noHome.env["ATHEL_HOME"] = ""
	if code, _ := noHome.runJSON("blast", "score", file); code != aerr.ExitState {
		t.Errorf("score without a home: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "blast.yaml"), []byte("schema: athel/blast-rules@1\nlevels: {critical-alert-firing: LOW}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("blast", "score", file); code != aerr.ExitPolicy {
		t.Errorf("loosening blast.yaml: exit %d", code)
	}
}
