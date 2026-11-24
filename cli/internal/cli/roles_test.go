package cli

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/engine"
)

// TestRoleCommandsExist keeps ADR 024's table equal to the CLI: every command a build role may
// run is a registered command.
func TestRoleCommandsExist(t *testing.T) {
	cmds := commands()
	for role, names := range engine.RoleCommands {
		for _, name := range names {
			if _, ok := cmds[name]; !ok {
				t.Errorf("%s may run athel %s, which does not exist", role, name)
			}
		}
	}
}

// TestSingleCommandsMatchCLI: the classifier names a one-word command by its word alone, so the
// role tables can list it; its list must be the CLI's one-word commands.
func TestSingleCommandsMatchCLI(t *testing.T) {
	for name := range commands() {
		if !strings.Contains(name, " ") && !classify.SingleCommands[name] {
			t.Errorf("athel %s is one word but not in classify.SingleCommands", name)
		}
	}
	for name := range commands() {
		if words := strings.Fields(name); len(words) == 3 && !classify.ThreeWordGroups[words[0]+" "+words[1]] {
			t.Errorf("athel %s has three words but its group is not in classify.ThreeWordGroups", name)
		}
	}
	for name := range classify.SingleCommands {
		if _, ok := commands()[name]; !ok {
			t.Errorf("classify.SingleCommands names %s, which the CLI does not have", name)
		}
	}
}
