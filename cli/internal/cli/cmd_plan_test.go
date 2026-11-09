package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestPlanAndMigrateCommands(t *testing.T) {
	h := setup(t)
	planPath := filepath.Join(t.TempDir(), "plan.yaml")
	if err := os.WriteFile(planPath, []byte("schema: athel/plan@1\nchange: CHG-001\nhash: null\ntasks:\n  - {id: T-01, title: x, repo: r, paths: [src/**], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: []}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("plan", "show", "CHG-001"); code != aerr.ExitState || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("show without plan: %d %v", code, env)
	}
	v := h.mustOK("plan", "set", "CHG-001", planPath)
	if v["plan"].(map[string]any)["hash"] == nil || v["approved"] != false {
		t.Errorf("plan set = %v", v)
	}
	if code, _ := h.runJSON("plan", "set", "CHG-001", filepath.Join(t.TempDir(), "missing")); code != aerr.ExitState {
		t.Errorf("missing file: exit %d", code)
	}
	if code, _ := h.runJSON("plan", "show", "CHG-404"); code != aerr.ExitState {
		t.Errorf("missing change: exit %d", code)
	}
	if code, _ := h.runJSON("plan", "set", "CHG-001"); code != aerr.ExitUsage {
		t.Errorf("usage: exit %d", code)
	}
	h.env["ATHEL_SESSION_ID"] = ""
	if code, _ := h.runJSON("plan", "set", "CHG-001", planPath); code != aerr.ExitState {
		t.Errorf("no session: exit %d", code)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "plan.yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("plan", "show", "CHG-001"); code != aerr.ExitState {
		t.Errorf("corrupt plan: exit %d", code)
	}
}

func TestMigrateCommand(t *testing.T) {
	h := setup(t)
	cf := filepath.Join(h.root, ".athel", "changes", "CHG-001", "change.yaml")
	b, _ := os.ReadFile(cf)
	legacy := regexp.MustCompile(`(?m)^integrity:\n(  .*\n)*`).ReplaceAllString(strings.Replace(string(b), "change@3", "change@1", 1), "")
	if err := os.WriteFile(cf, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(h.root, ".athel", "config", "athel.yaml")
	if err := os.WriteFile(project, []byte("# kept\nschema: athel/config@1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m := h.mustOK("migrate", "--dry-run"); len(m["migrated"].([]any)) != 1 || len(m["config_files"].([]any)) != 1 || m["dry_run"] != true {
		t.Errorf("dry run = %v", m)
	}
	if m := h.mustOK("migrate"); len(m["migrated"].([]any)) != 1 || m["config_files"].([]any)[0] != project {
		t.Errorf("migrate = %v", m)
	}
	if b, _ := os.ReadFile(project); string(b) != "# kept\nschema: athel/config@2\n" {
		t.Errorf("project layer = %q", b)
	}
	if err := os.WriteFile(project, []byte("{schema: athel/config@1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("migrate"); code != aerr.ExitPolicy {
		t.Errorf("unmigratable project layer: exit %d", code)
	}
	if err := os.WriteFile(project, []byte("schema: athel/config@2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("migrate", "x"); code != aerr.ExitUsage {
		t.Errorf("usage: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("migrate"); code != aerr.ExitState {
		t.Errorf("outside workspace: exit %d", code)
	}
}
