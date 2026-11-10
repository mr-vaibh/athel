package plan

import (
	"os"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestTemplateIsValidAndLintClean(t *testing.T) {
	p, err := Decode(schema.NewRegistry(), []byte(Template))
	if err != nil {
		t.Fatal(err)
	}
	if problems := Lint(p, LintContext{Criteria: []string{"AC-1"}, Risk: "HIGH"}); len(problems) != 0 {
		t.Errorf("the template fails its own lint: %v", problems)
	}
}

// TestArchitectListsTheTaskKinds keeps the architect's task kinds equal to plan@2's.
func TestArchitectListsTheTaskKinds(t *testing.T) {
	b, err := os.ReadFile("../../../plugin/agents/architect.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{KindExpand, KindMigrate, KindConsume, KindContract, KindInternal} {
		if !strings.Contains(string(b), "`"+k+"`") {
			t.Errorf("architect.md does not describe kind %s", k)
		}
	}
}
