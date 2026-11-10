package prd

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Review is prd/review-<version>.md's block (athel/prd-review@1): the architect's completeness
// checklist (FR-SPEC-001), ambiguities and contradictions (FR-SPEC-002), and assumptions.
type Review struct {
	Schema         string               `yaml:"schema"`
	Change         string               `yaml:"change"`
	Version        string               `yaml:"version"`
	Checklist      map[string]CheckItem `yaml:"checklist"`
	Ambiguities    []Ambiguity          `yaml:"ambiguities"`
	Contradictions []Contradiction      `yaml:"contradictions"`
	Assumptions    []Assumption         `yaml:"assumptions"`
}

// CheckItem is one checklist entry.
type CheckItem struct {
	Status string   `yaml:"status"`
	PRD    []string `yaml:"prd"`
	Note   string   `yaml:"note,omitempty"`
}

// Ambiguity is a vague phrase and how it was resolved.
type Ambiguity struct {
	PRD        string `yaml:"prd"`
	Phrase     string `yaml:"phrase"`
	Material   bool   `yaml:"material"`
	Resolution string `yaml:"resolution"`
	Ref        string `yaml:"ref,omitempty"`
}

// Contradiction is two or more PRD statements that cannot all hold.
type Contradiction struct {
	PRD  []string `yaml:"prd"`
	Text string   `yaml:"text"`
	Card string   `yaml:"card"`
}

// Assumption is a stated assumption for a non-material gap.
type Assumption struct {
	ID   string   `yaml:"id"`
	Text string   `yaml:"text"`
	PRD  []string `yaml:"prd"`
}

// ChecklistOrder is doc 05 FR-SPEC-001's checklist, in its order.
var ChecklistOrder = []string{"goal", "actors", "scope", "out_of_scope", "functional_requirements",
	"acceptance_criteria", "non_functional", "constraints", "dependencies"}

// DecodeReview reads a review block strictly and validates it against prd-review@1.
func DecodeReview(reg *schema.Registry, data []byte) (*Review, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var r Review
	if err := dec.Decode(&r); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode PRD review", err)
	}
	if err := reg.ValidateYAML(schema.PRDReview, data); err != nil {
		return nil, err
	}
	return &r, nil
}

// CheckReview lists what the schema cannot check: the review is for this change and version,
// every PRD ID it cites exists, every material ambiguity is resolved, and every assumption it
// refers to is stated.
func CheckReview(r *Review, x Index) []string {
	var problems []string
	if r.Change != x.Change || r.Version != x.Version {
		problems = append(problems, fmt.Sprintf("the review is for %s %s, not %s %s", r.Change, r.Version, x.Change, x.Version))
	}
	unknown := func(where string, ids ...string) {
		for _, id := range ids {
			if !x.Has(id) {
				problems = append(problems, where+" cites "+id+", which the PRD index does not have")
			}
		}
	}
	for _, k := range ChecklistOrder {
		unknown("checklist "+k, r.Checklist[k].PRD...)
	}
	assumed := map[string]bool{}
	for _, a := range r.Assumptions {
		unknown("assumption "+a.ID, a.PRD...)
		assumed[a.ID] = true
	}
	for _, a := range r.Ambiguities {
		unknown("ambiguity \""+a.Phrase+"\"", a.PRD)
		switch {
		case a.Material && a.Resolution == "none":
			problems = append(problems, "the material ambiguity \""+a.Phrase+"\" in "+a.PRD+" needs a card or an assumption")
		case a.Resolution == "assumption" && !assumed[a.Ref]:
			problems = append(problems, "ambiguity \""+a.Phrase+"\" refers to assumption "+a.Ref+", which is not stated")
		case a.Resolution == "card" && !strings.HasPrefix(a.Ref, "D-"):
			problems = append(problems, "ambiguity \""+a.Phrase+"\" is resolved by a card but names none")
		}
	}
	for _, c := range r.Contradictions {
		unknown("contradiction", c.PRD...)
	}
	return problems
}

// Missing lists the checklist items not fully present, for the reader and the spec step.
func (r *Review) Missing() []string {
	var out []string
	for _, k := range ChecklistOrder {
		if st := r.Checklist[k].Status; st != "present" {
			out = append(out, k+" ("+st+")")
		}
	}
	return out
}

// RenderReview writes prd/review-<version>.md: a short summary for the reader, then the block.
func RenderReview(r *Review, block []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# PRD review · %s · %s\n\n", r.Change, r.Version)
	if m := r.Missing(); len(m) > 0 {
		b.WriteString("**Missing or partial:** " + strings.Join(m, ", ") + "\n\n")
	} else {
		b.WriteString("**Checklist:** every item present.\n\n")
	}
	material := 0
	for _, a := range r.Ambiguities {
		if a.Material {
			material++
		}
	}
	fmt.Fprintf(&b, "**Ambiguities:** %d (%d material). **Contradictions:** %d. **Assumptions:** %d.\n\n",
		len(r.Ambiguities), material, len(r.Contradictions), len(r.Assumptions))
	b.WriteString("## Review block (athel/prd-review@1, checked by the athel CLI)\n\n```yaml\n")
	b.Write(bytes.TrimRight(block, "\n"))
	b.WriteString("\n```\n")
	return b.Bytes()
}
