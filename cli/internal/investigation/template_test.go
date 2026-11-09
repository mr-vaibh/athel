package investigation

import (
	"encoding/json"
	"strings"
	"testing"

	athel "github.com/mr-vaibh/athel"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestTemplateIsValid(t *testing.T) {
	inv, err := Decode(schema.NewRegistry(), []byte(Template))
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Claims) != 1 || inv.Ask != "ASK-001" {
		t.Errorf("template decodes to %+v", inv)
	}
}

// TestTemplateListsEveryValue keeps the comments in step with the schema: every enum value the
// schema allows appears in the template.
func TestTemplateListsEveryValue(t *testing.T) {
	b, err := athel.Schemas.ReadFile("schemas/investigation@1.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Properties struct {
			Type struct {
				Enum []string `json:"enum"`
			} `json:"type"`
		} `json:"properties"`
		Defs map[string]struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	lists := map[string][]string{
		"type":        doc.Properties.Type.Enum,
		"predicate":   doc.Defs["claim"].Properties["predicate"].Enum,
		"entity kind": doc.Defs["entity"].Properties["kind"].Enum,
		"source kind": doc.Defs["source"].Properties["kind"].Enum,
	}
	for name, values := range lists {
		if len(values) == 0 {
			t.Fatalf("schema has no %s enum", name)
		}
		for _, v := range values {
			if !strings.Contains(Template, " "+v+" ") && !strings.Contains(Template, " "+v+"\n") {
				t.Errorf("%s %q is not listed in the template", name, v)
			}
		}
	}
}
