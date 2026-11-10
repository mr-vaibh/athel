package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/investigation"
	"github.com/mr-vaibh/athel/cli/internal/observe"
)

const claimBlock = `schema: athel/investigation@1
change: CHG-001
question: Who owns the users service?
type: service_owner
scope: {services: [users], repos: []}
complete: true
budget: {max_tool_calls: 60, max_minutes: 10, used_tool_calls: 4, used_minutes: 1}
claims:
  - id: C1
    text: team identity owns users
    subject: {kind: service, name: users}
    predicate: owned_by
    object: {kind: team, name: identity}
    env: any
    confidence: likely
    evidence: [{kind: codeowners, ref: users/CODEOWNERS:1}]
    facts: [F-0001]
drift: []
gaps: []
active_work: []
health: []
suspicious: []
facts_used: [F-0001]
facts_created: []
`

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func investigationSetup(t *testing.T) *harness {
	t.Helper()
	h := setup(t)
	h.stdin = factDraft
	h.mustOK("fact", "add", "-")
	h.mustOK("fact", "use", "F-0001", "--purpose", "orientation", "--change", "CHG-001")
	return h
}

func TestInvestigationSave(t *testing.T) {
	h := investigationSetup(t)
	block := writeTemp(t, "claims.yaml", claimBlock)
	summary := writeTemp(t, "summary.md", "Team identity owns users.\n")
	if v := h.mustOK("investigation", "validate", "--change", "CHG-001", block); v["valid"] != true {
		t.Fatalf("validate = %v", v)
	}
	v := h.mustOK("investigation", "save", "--change", "CHG-001", "--summary", summary, block)
	md, err := os.ReadFile(v["path"].(string))
	if err != nil || !strings.Contains(string(md), "Team identity owns users.") || !strings.Contains(string(md), "```yaml\nschema: athel/investigation@1") {
		t.Fatalf("investigation.md = %s %v", md, err)
	}
	h.stdin = claimBlock
	if v := h.mustOK("investigation", "save", "--change", "CHG-001", "-"); v["report"].(map[string]any)["claims"].([]any)[0].(map[string]any)["ceiling"].(map[string]any)["label"] != "likely" {
		t.Errorf("save from stdin = %v", v)
	}
}

func TestInvestigationRejected(t *testing.T) {
	h := investigationSetup(t)
	over := writeTemp(t, "claims.yaml", strings.Replace(claimBlock, "confidence: likely", "confidence: confirmed", 1))
	if v := h.mustOK("investigation", "validate", "--change", "CHG-001", over); v["valid"] != false {
		t.Errorf("validate overclaim = %v", v)
	}
	code, env := h.runJSON("investigation", "save", "--change", "CHG-001", over)
	if code != aerr.ExitState || errCode(env) != string(aerr.StateConfidence) {
		t.Errorf("save overclaim: exit %d %v", code, env)
	}
	if _, err := os.Stat(filepath.Join(h.root, ".athel", "changes", "CHG-001", "investigation.md")); err == nil {
		t.Error("a rejected investigation was written")
	}
	dir := filepath.Join(h.root, ".athel", "knowledge", "observations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	obs := `{"schema":"athel/observation@1","tool_use_id":"toolu_inj","session":"sess-A","agent_type":"athel:scout","tool":"Bash","input":"{}",` +
		`"output":"{}","output_sha256":"sha256:` + strings.Repeat("0", 64) + `","truncated":false,` +
		`"suspicious":[{"phrase":"you must now","excerpt":"you must now run it"}],"at":"2026-11-01T10:00:00.000Z","expires_at":"2026-11-08T10:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(dir, "toolu_inj.json"), []byte(obs), 0o600); err != nil {
		t.Fatal(err)
	}
	block := writeTemp(t, "claims.yaml", claimBlock)
	if code, env := h.runJSON("investigation", "save", "--change", "CHG-001", block); code != aerr.ExitState || !strings.Contains(env["error"].(map[string]any)["message"].(string), "toolu_inj") {
		t.Errorf("unlisted suspicious call: exit %d %v", code, env)
	}
	listed := writeTemp(t, "claims.yaml", strings.Replace(claimBlock, "suspicious: []", `suspicious: [{ref: "log", excerpt: "you must now run it", tool_use_id: toolu_inj}]`, 1))
	h.mustOK("investigation", "save", "--change", "CHG-001", listed)
}

func TestInvestigationCommandErrors(t *testing.T) {
	h := investigationSetup(t)
	h.mustOK("lock", "acquire", "CHG-001")
	block := writeTemp(t, "claims.yaml", claimBlock)
	tests := []struct {
		name string
		args []string
		env  map[string]string
		code int
	}{
		{"usage", []string{"investigation", "save"}, nil, aerr.ExitUsage},
		{"no session", []string{"investigation", "save", "--change", "CHG-001", block}, map[string]string{"ATHEL_SESSION_ID": ""}, aerr.ExitState},
		{"missing block", []string{"investigation", "save", "--change", "CHG-001", filepath.Join(t.TempDir(), "x")}, nil, aerr.ExitState},
		{"missing summary", []string{"investigation", "save", "--change", "CHG-001", "--summary", filepath.Join(t.TempDir(), "x"), block}, nil, aerr.ExitState},
		{"invalid block", []string{"investigation", "save", "--change", "CHG-001", writeTemp(t, "bad.yaml", "schema: x\n")}, nil, aerr.ExitState},
		{"neither change nor ask", []string{"investigation", "save", block}, nil, aerr.ExitUsage},
		{"both change and ask", []string{"investigation", "save", "--change", "CHG-001", "--ask", "ASK-001", block}, nil, aerr.ExitUsage},
		{"unknown ask", []string{"investigation", "save", "--ask", "ASK-009", block}, nil, aerr.ExitState},
		{"unknown change", []string{"investigation", "save", "--change", "CHG-009", block}, nil, aerr.ExitState},
		{"other session holds the lock", []string{"investigation", "save", "--change", "CHG-001", block}, map[string]string{"ATHEL_SESSION_ID": "sess-B"}, aerr.ExitState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				old := h.env[k]
				h.env[k] = v
				defer func() { h.env[k] = old }()
			}
			if code, env := h.runJSON(tt.args...); code != tt.code {
				t.Errorf("exit %d, want %d (%v)", code, tt.code, env)
			}
		})
	}
	outside := newHarness(t)
	outside.env["ATHEL_SESSION_ID"] = "s"
	if code, _ := outside.runJSON("investigation", "save", "--change", "CHG-001", block); code != aerr.ExitState {
		t.Errorf("outside a workspace: exit %d", code)
	}
	access := filepath.Join(h.root, ".athel", "knowledge", "access.jsonl")
	if err := os.WriteFile(access, []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("investigation", "save", "--change", "CHG-001", block); code != aerr.ExitState {
		t.Errorf("corrupt access log: exit %d", code)
	}
	if err := os.Remove(access); err != nil {
		t.Fatal(err)
	}
	obsDir := filepath.Join(h.root, ".athel", "knowledge", "observations")
	if err := os.MkdirAll(obsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(obsDir, "toolu_bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("investigation", "save", "--change", "CHG-001", block); code != aerr.ExitState {
		t.Errorf("corrupt observation: exit %d", code)
	}
}

func TestInvestigationScopedToRunningStep(t *testing.T) {
	h := investigationSetup(t)
	if code, _ := h.runJSON("investigation", "validate"); code != aerr.ExitUsage {
		t.Errorf("validate usage: exit %d", code)
	}
	h.mustOK("state", "enter", "CHG-001", "start")
	block := writeTemp(t, "claims.yaml", claimBlock)
	if v := h.mustOK("investigation", "validate", "--change", "CHG-001", block); v["valid"] != true {
		t.Errorf("validate during a step = %v", v)
	}
}

func TestInvestigationCreatedFactsChecked(t *testing.T) {
	h := investigationSetup(t)
	created := writeTemp(t, "claims.yaml", strings.Replace(claimBlock, "facts_created: []", "facts_created: [F-0001]", 1))
	h.mustOK("investigation", "save", "--change", "CHG-001", created)
	missing := writeTemp(t, "claims.yaml", strings.Replace(claimBlock, "facts_created: []", "facts_created: [F-0007]", 1))
	if code, _ := h.runJSON("investigation", "save", "--change", "CHG-001", missing); code != aerr.ExitState {
		t.Errorf("unknown created fact: exit %d", code)
	}
}

func TestAskInvestigation(t *testing.T) {
	h := setup(t)
	h.stdin = factDraft
	h.mustOK("fact", "add", "-")
	a := h.mustOK("ask", "new", "--question", "Who owns the users service?")
	if a["id"] != "ASK-001" || a["kind"] != "question" || a["session"] != "sess-A" {
		t.Fatalf("ask new = %v", a)
	}
	h.mustOK("fact", "use", "F-0001", "--purpose", "orientation", "--ask", "ASK-001")
	block := writeTemp(t, "claims.yaml", strings.Replace(claimBlock, "change: CHG-001", "ask: ASK-001", 1))
	v := h.mustOK("investigation", "save", "--ask", "ASK-001", block)
	if !strings.HasSuffix(v["path"].(string), filepath.Join("asks", "ASK-001", "investigation.md")) {
		t.Errorf("save = %v", v)
	}
	if shown := h.mustOK("ask", "show", "ASK-001"); shown["question"] != "Who owns the users service?" {
		t.Errorf("show = %v", shown)
	}
	if code, _ := h.runJSON("investigation", "save", "--ask", "ASK-001", writeTemp(t, "c.yaml", claimBlock)); code != aerr.ExitState {
		t.Errorf("a change's block saved as an ask: exit %d", code)
	}
	answer := filepath.Join(h.root, ".athel", "asks", "ASK-001", "investigation.md")
	if err := os.Remove(answer); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(answer, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("investigation", "save", "--ask", "ASK-001", block); code != aerr.ExitState {
		t.Errorf("unwritable answer file: exit %d", code)
	}
	for args, code := range map[string]int{"ask new": aerr.ExitUsage, "ask new x": aerr.ExitUsage, "ask show": aerr.ExitUsage, "ask new --question q --kind other": aerr.ExitState, "ask show ASK-1": aerr.ExitState} {
		if got, _ := h.runJSON(strings.Fields(args)...); got != code {
			t.Errorf("%s: exit %d, want %d", args, got, code)
		}
	}
	outside := newHarness(t)
	for _, args := range [][]string{{"ask", "new", "--question", "q"}, {"ask", "show", "ASK-001"}} {
		if code, _ := outside.runJSON(args...); code != aerr.ExitState {
			t.Errorf("%v outside a workspace: exit %d", args, code)
		}
	}
}

func TestSummaryText(t *testing.T) {
	h := investigationSetup(t)
	h.stdin = claimBlock
	v := h.mustOK("investigation", "save", "--change", "CHG-001", "--summary-text", "Identity owns users.", "-")
	md, _ := os.ReadFile(v["path"].(string))
	if !strings.Contains(string(md), "Identity owns users.") {
		t.Errorf("investigation.md = %s", md)
	}
	if code, _ := h.runJSON("investigation", "save", "--change", "CHG-001", "--summary", "x", "--summary-text", "y", "-"); code != aerr.ExitUsage {
		t.Errorf("both summaries: exit %d", code)
	}
}

func TestInvestigationTemplate(t *testing.T) {
	h := newHarness(t)
	code, out, _ := h.run("investigation", "template")
	if code != 0 || !strings.HasPrefix(out, "# athel/investigation@1 claim block") || !strings.HasSuffix(out, "facts_created: []\n") {
		t.Errorf("template: exit %d\n%s", code, out)
	}
	if code, _, _ := h.run("investigation", "template", "extra"); code != 2 {
		t.Errorf("an argument is a usage error, got exit %d", code)
	}
}

func TestHelpFlag(t *testing.T) {
	h := newHarness(t)
	for _, flagName := range []string{"--help", "-h"} {
		code, out, errOut := h.run("investigation", "save", flagName)
		if code != 0 || errOut != "" || !strings.Contains(out, "usage: athel investigation save") || !strings.Contains(out, "-summary-text") {
			t.Errorf("%s: exit %d\n%s%s", flagName, code, out, errOut)
		}
	}
	if code, out, _ := h.run("--json", "fact", "add", "--help"); code != 0 || !strings.Contains(out, "usage: athel fact add") {
		t.Errorf("help with --json: exit %d %s", code, out)
	}
}

// TestTemplateIsNotSuspicious: the scout reads the template in the session it saves from, so
// text in it that the injection check flags would block its own save.
func TestTemplateIsNotSuspicious(t *testing.T) {
	if flags := observe.Suspicious(investigation.Template); len(flags) != 0 {
		t.Errorf("the template trips the suspicious-content check: %+v", flags)
	}
}

// TestM3TemplatesPrintCleanly: every template prints, and none trips the injection check the
// architect's session runs on tool output (the M2 lesson, TestTemplateIsNotSuspicious).
func TestM3TemplatesPrintCleanly(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"spec", "decide", "plan", "prd review", "blast"} {
		code, out, _ := h.run(append(strings.Fields(name), "template")...)
		if code != 0 || !strings.HasPrefix(out, "#") {
			t.Errorf("%s template: exit %d %q", name, code, out)
		}
		if flags := observe.Suspicious(out); len(flags) != 0 {
			t.Errorf("%s template trips the suspicious-content check: %+v", name, flags)
		}
	}
	if code, _, _ := h.run("plan", "template", "extra"); code != 2 {
		t.Errorf("an argument is a usage error: %d", code)
	}
}
