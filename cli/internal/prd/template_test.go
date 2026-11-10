package prd

import (
	"os"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// templatePRD is the unrelated PRD the review template's IDs refer to.
const templatePRD = `# Invoice downloads
Customers must be able to download their invoices.
Downloads should be fast.
## Limits
An invoice must be downloadable for seven years.
Invoices older than five years must be archived and not downloadable.
`

func TestReviewTemplateIsValidAndChecksClean(t *testing.T) {
	r, err := DecodeReview(schema.NewRegistry(), []byte(ReviewTemplate))
	if err != nil {
		t.Fatal(err)
	}
	x := Build(r.Change, r.Version, "sha256:x", []byte(templatePRD), nil)
	if p := CheckReview(r, x); len(p) != 0 {
		t.Errorf("the template fails the CLI's own checks: %v", p)
	}
	for _, want := range []string{"present", "partial", "missing", "card", "assumption", "none"} {
		if !strings.Contains(ReviewTemplate, want) {
			t.Errorf("the template does not show the allowed value %q", want)
		}
	}
}

// TestArchitectStartsFromTheReviewTemplate: the first M3 smoke run that got past onboarding spent
// 12 of the architect's turns guessing the review's format.
func TestArchitectStartsFromTheReviewTemplate(t *testing.T) {
	b, err := os.ReadFile("../../../plugin/agents/architect.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "athel prd review template") {
		t.Error("architect.md must start the PRD review from athel prd review template")
	}
}
