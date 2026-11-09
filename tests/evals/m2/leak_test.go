package m2

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureNames are names only the shop fixture uses. The plugin the scout reads (agents, skills,
// playbooks) and the CLI's claim block template must never contain them: an example that matches
// a trap would hand the scout the answer and inflate the eval (the first paid run found the
// playbooks citing `user-svc` vs `users`, PR 812 and users.updated.v1).
var fixtureNames = []string{"acme", "user-svc", "user-service", "users.updated", "phone", "legacy-profile",
	"812", "SHOP-", "shopmetrics", "commerce", "identity", "notifications", "teammate"}

func TestPluginNeverHintsAtTheFixture(t *testing.T) {
	roots := []string{"../../../plugin/agents", "../../../plugin/skills", "../../../cli/internal/investigation/template.yaml"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, name := range fixtureNames {
				if strings.Contains(strings.ToLower(string(b)), strings.ToLower(name)) {
					t.Errorf("%s mentions the fixture's %q", p, name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
