package cli

import (
	"testing"

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
