package prd

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const reviewDoc = `schema: athel/prd-review@1
change: CHG-001
version: v1
checklist:
  goal: {status: present, prd: [PRD-1.1]}
  actors: {status: present, prd: [PRD-1.1]}
  scope: {status: present, prd: [PRD-1]}
  out_of_scope: {status: missing, prd: []}
  functional_requirements: {status: present, prd: [PRD-1.2, PRD-1.3]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: partial, prd: [PRD-2.1]}
  constraints: {status: present, prd: [PRD-1.2]}
  dependencies: {status: present, prd: [PRD-1.3]}
ambiguities:
  - {prd: PRD-2.1, phrase: within 200 ms, material: true, resolution: card, ref: D-1}
  - {prd: PRD-1.1, phrase: able, material: false, resolution: assumption, ref: A-1}
contradictions: []
assumptions:
  - {id: A-1, text: Customers change only their own number, prd: [PRD-1.1]}
`

func v1Index(t *testing.T) Index {
	t.Helper()
	return Build("CHG-001", "v1", "sha256:x", []byte(v1), nil)
}

func TestReviewPasses(t *testing.T) {
	r, err := DecodeReview(schema.NewRegistry(), []byte(reviewDoc))
	if err != nil {
		t.Fatal(err)
	}
	if p := CheckReview(r, v1Index(t)); len(p) != 0 {
		t.Errorf("problems: %v", p)
	}
	md := string(RenderReview(r, []byte(reviewDoc)))
	for _, want := range []string{"# PRD review · CHG-001 · v1", "out_of_scope (missing), acceptance_criteria (missing), non_functional (partial)",
		"**Ambiguities:** 2 (1 material)", "```yaml\nschema: athel/prd-review@1"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q:\n%s", want, md)
		}
	}
}

func TestReviewProblems(t *testing.T) {
	reg := schema.NewRegistry()
	tests := map[string]struct{ from, to, want string }{
		"another version":       {"version: v1", "version: v2", "the review is for CHG-001 v2, not CHG-001 v1"},
		"unknown PRD ID":        {"goal: {status: present, prd: [PRD-1.1]}", "goal: {status: present, prd: [PRD-7.1]}", "checklist goal cites PRD-7.1"},
		"unresolved material":   {"material: true, resolution: card, ref: D-1", "material: true, resolution: none", "needs a card or an assumption"},
		"missing assumption":    {"resolution: assumption, ref: A-1", "resolution: assumption, ref: A-9", "assumption A-9, which is not stated"},
		"card without an ID":    {"resolution: card, ref: D-1", "resolution: card", "resolved by a card but names none"},
		"contradiction unknown": {"contradictions: []", "contradictions: [{prd: [PRD-1.1, PRD-9.9], text: x, card: D-2}]", "contradiction cites PRD-9.9"},
	}
	for name, tt := range tests {
		r, err := DecodeReview(reg, []byte(strings.Replace(reviewDoc, tt.from, tt.to, 1)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if p := strings.Join(CheckReview(r, v1Index(t)), "; "); !strings.Contains(p, tt.want) {
			t.Errorf("%s: %q lacks %q", name, p, tt.want)
		}
	}
	allPresent := strings.NewReplacer("status: missing", "status: present", "status: partial", "status: present").Replace(reviewDoc)
	if r, _ := DecodeReview(reg, []byte(allPresent)); !strings.Contains(string(RenderReview(r, nil)), "every item present") {
		t.Error("a complete checklist says so")
	}
	for name, doc := range map[string]string{"unknown field": reviewDoc + "extra: 1\n", "schema": strings.Replace(reviewDoc, "status: present", "status: maybe", 1)} {
		if _, err := DecodeReview(reg, []byte(doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
