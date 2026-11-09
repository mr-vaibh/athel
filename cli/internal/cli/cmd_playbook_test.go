package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestPlaybookCommands(t *testing.T) {
	h := newHarness(t)
	list := h.mustOK("playbook", "list")["tools"].([]any)
	if len(list) < 9 || list[0] != "gcloud" {
		t.Errorf("list = %v", list)
	}
	code, out, _ := h.run("playbook", "show", "kubectl")
	if code != 0 || !strings.HasPrefix(out, "---\ndescription:") || !strings.Contains(out, "## Read queries") {
		t.Errorf("show = %d %q", code, out)
	}
	for args, want := range map[string]int{"playbook show": aerr.ExitUsage, "playbook list x": aerr.ExitUsage,
		"playbook show nope": aerr.ExitState, "playbook show ../../go.mod": aerr.ExitState} {
		if got, _ := h.runJSON(strings.Fields(args)...); got != want {
			t.Errorf("%s: exit %d, want %d", args, got, want)
		}
	}
}

func TestProjectPlaybook(t *testing.T) {
	h := setup(t)
	cfg := filepath.Join(h.root, ".athel", "config")
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(cfg, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tools := func(playbook string) string {
		return "schema: athel/tools@1\ntools:\n  - {id: metrics, kind: mcp, match: {server: metrics}, purpose: [m], read: [q], write: [], playbook: \"" + playbook + "\"}\n"
	}
	write("tools.yaml", tools("playbooks/metrics.md"))
	write("playbooks/metrics.md", "# metrics\n")
	if code, out, _ := h.run("playbook", "show", "metrics"); code != 0 || out != "# metrics\n" {
		t.Errorf("show = %d %q", code, out)
	}
	for name, pb := range map[string]string{"traversal": "../../../go.mod", "missing file": "playbooks/none.md", "absolute": "/etc/hosts"} {
		write("tools.yaml", tools(pb))
		if code, _ := h.runJSON("playbook", "show", "metrics"); code != aerr.ExitState {
			t.Errorf("%s: exit %d", name, code)
		}
	}
	if code, _ := h.runJSON("playbook", "show", "unknown-tool"); code != aerr.ExitState {
		t.Errorf("unknown tool: exit %d", code)
	}
	write("tools.yaml", "schema: athel/tools@1\ntools: [{id: x}]\n")
	if code, _ := h.runJSON("playbook", "show", "metrics"); code != aerr.ExitPolicy {
		t.Errorf("invalid tools.yaml: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("playbook", "show", "metrics"); code != aerr.ExitState {
		t.Errorf("outside a workspace: exit %d", code)
	}
}
