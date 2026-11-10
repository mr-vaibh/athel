package decisions

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestTemplateIsAValidDraft(t *testing.T) {
	var d Draft
	dec := yaml.NewDecoder(bytes.NewReader([]byte(Template)))
	dec.KnownFields(true)
	if err := dec.Decode(&d); err != nil {
		t.Fatal(err)
	}
	f := New("CHG-001")
	if _, err := f.Add(d); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Encode(schema.NewRegistry()); err != nil {
		t.Errorf("the template's card does not validate: %v", err)
	}
}

// TestArchitectListsTheAskRules keeps the architect's ask-rule table equal to decision@1's rules.
func TestArchitectListsTheAskRules(t *testing.T) {
	b, err := os.ReadFile("../../../plugin/agents/architect.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{"A1", "A2", "A3", "A4", "A5", "A6", "A7"} {
		if !strings.Contains(string(b), "| "+r+" |") {
			t.Errorf("architect.md has no row for %s", r)
		}
	}
	if strings.Contains(string(b), "| A8 |") {
		t.Error("architect.md lists a rule decision@1 does not allow")
	}
}
