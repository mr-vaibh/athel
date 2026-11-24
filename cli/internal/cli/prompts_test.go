package cli

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/engine"
)

// TestAgentPromptsNameOnlyTheirCommands: each build agent's prompt names only athel commands
// that exist and that its role may run (ADR 024), so the prompt never leads it into a denial.
func TestAgentPromptsNameOnlyTheirCommands(t *testing.T) {
	cmds := commands()
	word := regexp.MustCompile("`athel (?:--json )?([a-z]+)(?: ([a-z]+))?(?: ([a-z]+))?")
	for role := range engine.RoleCommands {
		name := strings.TrimPrefix(role, "athel:")
		b, err := os.ReadFile("../../../plugin/agents/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "\nname: "+name+"\n") {
			t.Errorf("%s.md is not named %s", name, name)
		}
		seen := 0
		for _, m := range word.FindAllStringSubmatch(string(b), -1) {
			cmd := strings.TrimSpace(m[1] + " " + m[2])
			if three := cmd + " " + m[3]; m[3] != "" {
				if _, ok := cmds[three]; ok {
					cmd = three
				}
			}
			if _, ok := cmds[cmd]; !ok {
				t.Errorf("%s.md names athel %s, which does not exist", name, cmd)
				continue
			}
			seen++
			if !slices.Contains(engine.RoleCommands[role], cmd) && writes(t, cmd) {
				t.Errorf("%s.md names athel %s, which its role may not run", name, cmd)
			}
		}
		for _, cmd := range engine.RoleCommands[role] {
			if !strings.Contains(string(b), "athel "+cmd) && !strings.Contains(string(b), "athel --json "+cmd) {
				t.Errorf("%s.md never says how to run athel %s", name, cmd)
			}
		}
		if seen == 0 {
			t.Errorf("%s.md names no athel command", name)
		}
	}
}

func writes(t *testing.T, cmd string) bool {
	t.Helper()
	in, err := json.Marshal(map[string]string{"command": "athel " + cmd + " CHG-001"})
	if err != nil {
		t.Fatal(err)
	}
	cr := classify.Classify(classify.Call{Tool: "Bash", Input: in, Cwd: "/"}, nil, classify.Options{})
	return len(cr.Actions) != 1 || classify.IsWrite(cr.Actions[0].Tier)
}
