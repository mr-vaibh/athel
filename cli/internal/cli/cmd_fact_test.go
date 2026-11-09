package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const factDraft = `subject: {kind: service, name: users}
predicate: owned_by
object: {kind: team, name: identity}
env: any
confidence: likely
sources: [{kind: codeowners, ref: users/CODEOWNERS:1}]
verify: [{tool: git, action: grep, repo: users, pattern: "@acme/identity"}]
freshness_class: organisational
`

func TestFactCommands(t *testing.T) {
	h := setup(t)
	h.stdin = factDraft
	added := h.mustOK("fact", "add", "-")
	f := added["fact"].(map[string]any)
	if f["id"] != "F-0001" || added["created"] != true || f["status"] != "active" {
		t.Fatalf("add = %v", added)
	}
	file := filepath.Join(t.TempDir(), "draft.yaml")
	if err := os.WriteFile(file, []byte(factDraft), 0o600); err != nil {
		t.Fatal(err)
	}
	if again := h.mustOK("fact", "add", file); again["created"] != false || again["fact"].(map[string]any)["id"] != "F-0001" {
		t.Errorf("re-add = %v", again)
	}
	if found := h.mustOK("fact", "find", "--subject", "users", "--predicate", "owned_by"); len(found["facts"].([]any)) != 1 {
		t.Errorf("find = %v", found)
	}
	if found := h.mustOK("fact", "find", "--env", "prod"); len(found["facts"].([]any)) != 0 {
		t.Errorf("find prod = %v", found)
	}
	if shown := h.mustOK("fact", "show", "F-0001"); shown["archived"] != false || shown["fact"].(map[string]any)["predicate"] != "owned_by" {
		t.Errorf("show = %v", shown)
	}
}

func TestFactUseAndArchiveCommands(t *testing.T) {
	h := setup(t)
	h.stdin = factDraft
	h.mustOK("fact", "add", "-")
	used := h.mustOK("fact", "use", "F-0001", "--purpose", "decision", "--change", "CHG-001")
	if used["result"] != "in_window" || used["fresh"] != true {
		t.Errorf("use = %v", used)
	}
	h.clock.Advance(8 * 24 * time.Hour)
	if code, env := h.runJSON("fact", "use", "F-0001", "--purpose", "decision", "--change", "CHG-001"); code != aerr.ExitState || errCode(env) != string(aerr.StateFactStale) {
		t.Errorf("stale decision use: exit %d %v", code, env)
	}
	if v := h.mustOK("fact", "archive", "--dry-run"); v["dry_run"] != true || len(v["archived"].([]any)) != 0 {
		t.Errorf("archive dry run = %v", v)
	}
	h.clock.Advance(91 * 24 * time.Hour)
	if v := h.mustOK("fact", "archive"); len(v["archived"].([]any)) != 1 {
		t.Errorf("archive = %v", v)
	}
	if shown := h.mustOK("fact", "show", "F-0001"); shown["archived"] != true {
		t.Errorf("show archived = %v", shown)
	}
}

func TestFactCommandErrors(t *testing.T) {
	h := setup(t)
	tests := []struct {
		name  string
		stdin string
		args  []string
		code  int
	}{
		{"add usage", "", []string{"fact", "add"}, aerr.ExitUsage},
		{"find usage", "", []string{"fact", "find", "x"}, aerr.ExitUsage},
		{"show usage", "", []string{"fact", "show"}, aerr.ExitUsage},
		{"use usage", "", []string{"fact", "use"}, aerr.ExitUsage},
		{"archive usage", "", []string{"fact", "archive", "x"}, aerr.ExitUsage},
		{"missing file", "", []string{"fact", "add", filepath.Join(t.TempDir(), "none.yaml")}, aerr.ExitState},
		{"draft with an id", "id: F-0001\n" + factDraft, []string{"fact", "add", "-"}, aerr.ExitState},
		{"draft without sources", "subject: {kind: service, name: x}\nverify: []\n", []string{"fact", "add", "-"}, aerr.ExitState},
		{"show unknown", "", []string{"fact", "show", "F-0042"}, aerr.ExitState},
		{"use without purpose", "", []string{"fact", "use", "F-0001", "--change", "CHG-001"}, aerr.ExitState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.stdin = tt.stdin
			if code, _ := h.runJSON(tt.args...); code != tt.code {
				t.Errorf("exit %d, want %d", code, tt.code)
			}
		})
	}
}

func TestFactCommandsNeedWorkspaceAndConfig(t *testing.T) {
	outside := newHarness(t)
	for _, args := range [][]string{{"fact", "find"}, {"fact", "show", "F-0001"}, {"fact", "use", "F-0001"}, {"fact", "archive"}} {
		if code, _ := outside.runJSON(args...); code != aerr.ExitState {
			t.Errorf("%v outside a workspace: exit %d", args, code)
		}
	}
	outside.stdin = factDraft
	if code, _ := outside.runJSON("fact", "add", "-"); code != aerr.ExitState {
		t.Errorf("add outside a workspace: exit %d", code)
	}
	h := setup(t)
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nbogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("fact", "find"); code != aerr.ExitPolicy {
		t.Errorf("invalid config: exit %d", code)
	}
}

func TestFactStoreErrorsReported(t *testing.T) {
	h := setup(t)
	knowledge := filepath.Join(h.root, ".athel", "knowledge")
	if err := os.MkdirAll(filepath.Join(knowledge, "facts.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.stdin = factDraft
	for _, args := range [][]string{{"fact", "add", "-"}, {"fact", "find"}, {"fact", "show", "F-0001"}, {"fact", "archive"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitState {
			t.Errorf("%v with an unreadable cache: exit %d", args, code)
		}
	}
}
