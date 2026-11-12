package spec

import (
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestTemplateIsValidAndLintClean(t *testing.T) {
	s, err := Decode(schema.NewRegistry(), []byte(Template))
	if err != nil {
		t.Fatal(err)
	}
	if p := Lint(s); len(p) != 0 {
		t.Errorf("the template fails its own lint: %v", p)
	}
}
