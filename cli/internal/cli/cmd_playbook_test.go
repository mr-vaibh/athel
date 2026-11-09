package cli

import (
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
