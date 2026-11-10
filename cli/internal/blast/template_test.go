package blast

import (
	"os"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestTemplateIsValidAndPlannedClean(t *testing.T) {
	r, err := Decode(schema.NewRegistry(), []byte(Template))
	if err != nil {
		t.Fatal(err)
	}
	if p := CheckPlanned(r, []string{"REQ-1", "REQ-2"}); len(p) != 0 {
		t.Errorf("the template fails the planned checks: %v", p)
	}
	if r.Risk != nil || len(r.RulesFired) != 0 {
		t.Error("the CLI computes the risk; the template leaves it null")
	}
}

// TestWritersStartFromTheTemplate: both writers of a report name the template, so neither guesses
// the format (the M3 smoke runs found the review's missing template costing a whole step).
func TestWritersStartFromTheTemplate(t *testing.T) {
	for _, p := range []string{"../../../plugin/agents/architect.md", "../../../plugin/skills/blast-radius/SKILL.md"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "blast template") {
			t.Errorf("%s does not start the report from athel blast template", p)
		}
	}
}
