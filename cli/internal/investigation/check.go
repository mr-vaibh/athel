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
// sources. Problems are rule violations; Mislabels are labels other than the one the evidence
// gives (ADR 018: the CLI decides labels, the scout states them). Hints never block a save:
// they point at evidence already in the block that may belong on another claim.
type Report struct {
	Claims    []ClaimCheck `json:"claims"`
	AddedGaps []Gap        `json:"added_gaps"`
	Mislabels []string     `json:"mislabels"`
	Problems  []string     `json:"problems"`
	Hints     []string     `json:"hints"`
}

// Err turns a report with violations into one error naming all of them: E-STATE-CONFIDENCE when
// a label differs from its evidence's (the scout must change it), E-STATE-INVALID otherwise.
func (r Report) Err() error {
	all := append(slices.Clone(r.Mislabels), r.Problems...)
	if len(all) == 0 {
		return nil
	}
	code := aerr.StateInvalid
	if len(r.Mislabels) > 0 {
		code = aerr.StateConfidence
	}
	return aerr.New(code, "investigation rejected: "+strings.Join(all, "; ")).
		WithNextAction("fix the claim block and save again; every label must be exactly what its evidence gives")
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
	r := Report{Claims: []ClaimCheck{}, AddedGaps: []Gap{}, Mislabels: []string{}, Problems: []string{}, Hints: []string{}}
	if inv.Change != ctx.Change || inv.Ask != ctx.Ask {
		r.Problems = append(r.Problems, fmt.Sprintf("the claim block is for %s%s, not %s%s", inv.Change, inv.Ask, ctx.Change, ctx.Ask))
	}
	r.checkClaims(inv)
	r.addMandatoryGaps(inv)
	r.hintIndependent(inv)
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
		if c.Confidence != ceil.Label {
			r.Mislabels = append(r.Mislabels, fmt.Sprintf("%s is labelled %s but its evidence gives %s (%s)", c.ID, c.Confidence, ceil.Label, ceil.Why))
		}
		if p := direction(c); p != "" {
			r.Problems = append(r.Problems, p)
		}
	}
}

// roles holds, for predicates whose direction is easy to get backwards, the entity kinds allowed
// as subject and as object. A claim reads subject first: users.updated.v1 produced_by users.
var roles = map[string]struct{ subject, object []string }{
	"produced_by":   {[]string{"event", "topic"}, nil},
	"consumed_by":   {[]string{"event", "topic"}, nil},
	"owned_by":      {nil, []string{"team", "person"}},
	"on_call":       {nil, []string{"team", "person"}},
	"owns_data":     {[]string{"service", "workload", "repo", "team"}, nil},
	"writes_data":   {[]string{"service", "workload", "repo"}, nil},
	"reads_data":    {[]string{"service", "workload", "repo"}, nil},
	"reads_config":  {nil, []string{"config"}},
	"mounts_secret": {nil, []string{"secret"}},
}

// direction names a claim whose subject or object kind does not fit its predicate, which is how
// a claim written backwards shows (the first eval run wrote "users produced_by users.updated.v1").
func direction(c Claim) string {
	r, ok := roles[c.Predicate]
	if !ok {
		return ""
	}
	for _, side := range []struct {
		name  string
		kind  string
		allow []string
	}{{"subject", c.Subject["kind"], r.subject}, {"object", c.Object["kind"], r.object}} {
		if side.allow != nil && !slices.Contains(side.allow, side.kind) {
			return fmt.Sprintf("%s: %s needs its %s kind to be %s, not %s; a claim reads subject first",
				c.ID, c.Predicate, side.name, strings.Join(side.allow, " or "), side.kind)
		}
	}
	return ""
}

// addMandatoryGaps records, once per source, the mandatory sources no answering claim used, and
// hints where another claim already cites one (the eval runs showed the scout filing the second
// source of an answer on a side claim, which leaves the answer a label lower).
func (r *Report) addMandatoryGaps(inv *Investigation) {
	for _, c := range inv.Claims {
		for _, name := range missingMandatory(c, inv.Type) {
			if s, other, ok := citedElsewhere(inv, c, name); ok {
				r.Hints = append(r.Hints, fmt.Sprintf("%s answers the question without %s, but %s cites %s %s: if it also supports %s, list it there",
					c.ID, name, other, s.Kind, s.Ref, c.ID))
			}
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

// hintIndependent points an answering claim short of confirmed at a strong source on another
// claim from a source system it lacks: the second, independent source that would confirm it,
// filed elsewhere (the fourth eval run's prod version: cluster on the answer, manifest on an
// alias claim). A claim already hinted about a mandatory source gets no second hint.
func (r *Report) hintIndependent(inv *Investigation) {
	answering := questionTypes[inv.Type].predicates
	for _, c := range inv.Claims {
		hinted := slices.ContainsFunc(r.Hints, func(h string) bool { return strings.HasPrefix(h, c.ID+" ") })
		if hinted || !slices.Contains(answering, c.Predicate) || CeilingOf(c, inv.Type).Label == Confirmed ||
			slices.ContainsFunc(c.Evidence, Source.contradicts) {
			continue
		}
		have := map[string]bool{}
		for _, e := range c.Evidence {
			have[systemOf[e.Kind]] = true
		}
		if s, other, ok := strongElsewhere(inv, c, have); ok {
			r.Hints = append(r.Hints, fmt.Sprintf("%s is short of confirmed, but %s cites %s %s from another source system: if it also supports %s, list it there",
				c.ID, other, s.Kind, s.Ref, c.ID))
		}
	}
}

// strongElsewhere finds a strong, supporting source on another claim than c whose system c lacks.
func strongElsewhere(inv *Investigation, c Claim, have map[string]bool) (Source, string, bool) {
	for _, other := range inv.Claims {
		if other.ID == c.ID {
			continue
		}
		for _, s := range other.Evidence {
			if !weak[s.Kind] && !s.contradicts() && !have[systemOf[s.Kind]] {
				return s, other.ID, true
			}
		}
	}
	return Source{}, "", false
}

// citedElsewhere finds a source of the named mandatory group on another claim than c.
func citedElsewhere(inv *Investigation, c Claim, group string) (Source, string, bool) {
	var kinds []string
	for _, g := range questionTypes[inv.Type].mandatory {
		if g.name == group {
			kinds = g.kinds
		}
	}
	for _, other := range inv.Claims {
		if other.ID == c.ID {
			continue
		}
		for _, s := range other.Evidence {
			if slices.Contains(kinds, s.Kind) && !s.contradicts() {
				return s, other.ID, true
			}
		}
	}
	return Source{}, "", false
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
