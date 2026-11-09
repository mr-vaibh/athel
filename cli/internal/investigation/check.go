package investigation

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// MaxSummaryWords is doc 03 NFR-DISC-03: scout output stays under 2,000 words in context.
const MaxSummaryWords = 2000

// Context is what the checks need to know beyond the document.
type Context struct {
	Change, Ask string                           // the change or ad-hoc question the answer is for
	Accessed    func(fact string) bool           // whether the fact use was logged for this change or ask
	FactStatus  func(fact string) (string, bool) // a fact's status, or false when it does not exist
	Flagged     []string                         // tool_use_ids whose output the hook flagged as suspicious
}

// ClaimCheck is the verdict on one claim's confidence.
type ClaimCheck struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Ceiling Ceiling `json:"ceiling"`
}

// Report is the outcome of the checks. Gaps lists the gaps the CLI added for missing mandatory
// sources. Problems are rule violations; Overclaims are labels above their ceiling.
type Report struct {
	Claims     []ClaimCheck `json:"claims"`
	AddedGaps  []Gap        `json:"added_gaps"`
	Overclaims []string     `json:"overclaims"`
	Problems   []string     `json:"problems"`
}

// Err turns a report with violations into one error naming all of them: E-STATE-CONFIDENCE when
// a label is above its ceiling (the scout must lower it), E-STATE-INVALID for anything else.
func (r Report) Err() error {
	all := append(slices.Clone(r.Overclaims), r.Problems...)
	if len(all) == 0 {
		return nil
	}
	code := aerr.StateInvalid
	if len(r.Overclaims) > 0 {
		code = aerr.StateConfidence
	}
	return aerr.New(code, "investigation rejected: "+strings.Join(all, "; ")).
		WithNextAction("fix the claim block and save again; a label may never exceed what its evidence allows")
}

// Decode reads one investigation@1 document: a strict decode first (unknown fields, wrong types,
// empty input), then the schema (values, patterns, a second document).
func Decode(reg *schema.Registry, data []byte) (*Investigation, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var inv Investigation
	if err := dec.Decode(&inv); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode investigation", err)
	}
	if err := reg.ValidateYAML(SchemaID, data); err != nil {
		return nil, err
	}
	return &inv, nil
}

// Check applies every rule the schema cannot express and adds the gaps the CLI owes the reader.
// It changes inv only by appending those gaps.
func Check(inv *Investigation, ctx Context, summary string) Report {
	r := Report{Claims: []ClaimCheck{}, AddedGaps: []Gap{}, Overclaims: []string{}, Problems: []string{}}
	if inv.Change != ctx.Change || inv.Ask != ctx.Ask {
		r.Problems = append(r.Problems, fmt.Sprintf("the claim block is for %s%s, not %s%s", inv.Change, inv.Ask, ctx.Change, ctx.Ask))
	}
	r.checkClaims(inv)
	r.addMandatoryGaps(inv)
	r.checkFacts(inv, ctx)
	if !inv.Complete && len(inv.Gaps) == 0 {
		r.Problems = append(r.Problems, "an incomplete answer must list what is missing under gaps (FR-DISC-014)")
	}
	for _, id := range ctx.Flagged {
		if !slices.ContainsFunc(inv.Suspicious, func(s Suspicious) bool { return s.ToolUseID == id }) {
			r.Problems = append(r.Problems, "tool call "+id+" returned suspicious content that is not listed under suspicious (add an entry with tool_use_id: "+id+")")
		}
	}
	if n := len(strings.Fields(summary)); n > MaxSummaryWords {
		r.Problems = append(r.Problems, fmt.Sprintf("the summary has %d words; the limit is %d (put raw data in artifacts/)", n, MaxSummaryWords))
	}
	return r
}

func (r *Report) checkClaims(inv *Investigation) {
	seen := map[string]bool{}
	for _, c := range inv.Claims {
		if seen[c.ID] {
			r.Problems = append(r.Problems, "claim id "+c.ID+" is used twice")
		}
		seen[c.ID] = true
		ceil := CeilingOf(c, inv.Type)
		r.Claims = append(r.Claims, ClaimCheck{ID: c.ID, Label: c.Confidence, Ceiling: ceil})
		if rank[c.Confidence] > rank[ceil.Label] {
			r.Overclaims = append(r.Overclaims, fmt.Sprintf("%s is labelled %s but its evidence allows at most %s (%s)", c.ID, c.Confidence, ceil.Label, ceil.Why))
		}
	}
}

// addMandatoryGaps records, once per source, the mandatory sources no answering claim used.
func (r *Report) addMandatoryGaps(inv *Investigation) {
	for _, c := range inv.Claims {
		for _, name := range missingMandatory(c, inv.Type) {
			if slices.ContainsFunc(inv.Gaps, func(g Gap) bool { return g.Source == name }) {
				continue
			}
			g := Gap{Source: name, Code: "E-TOOL-SOURCE-MISSING", Reason: "no claim answering the question used " + name,
				Effect: "claims that needed it are at most likely"}
			inv.Gaps = append(inv.Gaps, g)
			r.AddedGaps = append(r.AddedGaps, g)
		}
	}
}

// checkFacts makes the access log complete (FR-DISC-004): every fact the answer relies on must
// have been read through `athel fact use` for this change or question, and every fact it says
// it created must exist.
func (r *Report) checkFacts(inv *Investigation, ctx Context) {
	cited := slices.Clone(inv.FactsUsed)
	for _, c := range inv.Claims {
		cited = append(cited, c.Facts...)
	}
	slices.Sort(cited)
	for _, id := range slices.Compact(cited) {
		if slices.Contains(inv.FactsCreated, id) {
			continue
		}
		if !ctx.Accessed(id) {
			r.Problems = append(r.Problems, id+" is cited but was not read with `athel fact use` for this question")
		}
	}
	for _, id := range inv.FactsCreated {
		if status, ok := ctx.FactStatus(id); !ok || status != "active" {
			r.Problems = append(r.Problems, id+" is listed as created but is not an active fact (add it with `athel fact add`)")
		}
	}
}
