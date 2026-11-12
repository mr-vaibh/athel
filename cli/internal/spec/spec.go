// Package spec reads and checks a service's spec (athel/spec@1, doc 05 §Spec format;
// FR-SPEC-003/004/005) and renders spec/<service>.md. The architect writes the block; the CLI
// checks the EARS form of every requirement, the testability of every acceptance criterion and
// every link back to the PRD and to decisions, so a spec cannot drift from the PRD unnoticed.
package spec

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Requirement is one EARS requirement.
type Requirement struct {
	ID        string   `yaml:"id" json:"id"`
	PRD       []string `yaml:"prd" json:"prd"`
	Text      string   `yaml:"text" json:"text"`
	Decisions []string `yaml:"decisions" json:"decisions"`
}

// Interface is a changed interface.
type Interface struct {
	Kind        string `yaml:"kind" json:"kind"`
	Name        string `yaml:"name" json:"name"`
	Change      string `yaml:"change" json:"change"`
	Description string `yaml:"description" json:"description"`
}

// DataChange is a changed table or column.
type DataChange struct {
	Table       string `yaml:"table" json:"table"`
	Change      string `yaml:"change" json:"change"`
	Description string `yaml:"description" json:"description"`
}

// Criterion is a testable acceptance criterion.
type Criterion struct {
	ID    string   `yaml:"id" json:"id"`
	Req   []string `yaml:"req" json:"req"`
	Given string   `yaml:"given" json:"given"`
	When  string   `yaml:"when" json:"when"`
	Then  string   `yaml:"then" json:"then"`
}

// Assumption is a stated assumption.
type Assumption struct {
	ID   string `yaml:"id" json:"id"`
	Text string `yaml:"text" json:"text"`
}

// Spec is spec/<service>.md's block.
type Spec struct {
	Schema       string        `yaml:"schema" json:"schema"`
	Change       string        `yaml:"change" json:"change"`
	Service      string        `yaml:"service" json:"service"`
	Repo         string        `yaml:"repo" json:"repo"`
	Requirements []Requirement `yaml:"requirements" json:"requirements"`
	Interfaces   []Interface   `yaml:"interfaces_changed" json:"interfaces_changed"`
	DataChanges  []DataChange  `yaml:"data_changes" json:"data_changes"`
	DesignNotes  []string      `yaml:"design_notes" json:"design_notes"`
	Criteria     []Criterion   `yaml:"acceptance_criteria" json:"acceptance_criteria"`
	OutOfScope   []string      `yaml:"out_of_scope" json:"out_of_scope"`
	Assumptions  []Assumption  `yaml:"assumptions" json:"assumptions"`
}

// Decode reads a spec block strictly and validates it against spec@1.
func Decode(reg *schema.Registry, data []byte) (*Spec, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode spec", err)
	}
	if err := reg.ValidateYAML(schema.Spec, data); err != nil {
		return nil, err
	}
	return &s, nil
}

// Context is what the checks compare a spec with.
type Context struct {
	Change   string
	PRD      func(id string) bool // whether the current PRD index has the ID
	Decision func(id string) bool // whether decisions.yaml has the ID
	Others   []*Spec              // the change's other services' specs (REQ and AC are unique per change)
}

var (
	// earsRe is the EARS shape: optional When/While/Where/If clauses, then "the <subject> shall".
	earsRe  = regexp.MustCompile(`(?i)^(?:(?:when|while|where|if)\b[^,]+,\s*(?:then\s+)?)*the\s+(.+?)\s+shall(?:\s+not)?\s+\S`)
	weakRe  = regexp.MustCompile(`(?i)\b(should|may|might|could|etc\.?|and/or|tbd|as appropriate|if possible)\b`)
	vagueRe = regexp.MustCompile(`(?i)^(?:it\s+|the\s+\w+\s+)?(?:works|is fast|is quick|succeeds|is ok|is correct|is handled|behaves correctly|is good)\.?$`)
	// observableRe is something a test can check: a status code, a returned, stored, published
	// or shown value, an error code, a count, or a quoted literal.
	observableRe = regexp.MustCompile(`(?i)(\b[1-5][0-9]{2}\b|\breturns?\b|\bresponds?\b|\bstored?\b|\bsaves?\b|\bpublish|\bemit|\blogs?\b|\brecord|\bshows?\b|\bdisplay|\bcontains?\b|\bequals?\b|\bincrement|\bdecrement|\bwrites?\b|\bsends?\b|\breceives?\b|\berror\b|\bcode\b|\bstatus\b|\bcount\b|"[^"]+"|\+?[0-9])`)
	bareNumberRe = regexp.MustCompile(`(?i)\b(within|under|below|over|above|at most|at least|less than|more than)\s+[0-9]+(?:\.[0-9]+)?\b(\s+(ms|milliseconds?|s|seconds?|minutes?|hours?|days?|%|percent|rows?|requests?|calls?|items?|mb|gb|kb|bytes?|times|retries))?`)
)

// Lint checks every requirement's EARS form and every criterion's testability (FR-SPEC-004/005).
func Lint(s *Spec) []string {
	var problems []string
	service := strings.ToLower(s.Service)
	for _, r := range s.Requirements {
		m := earsRe.FindStringSubmatch(r.Text)
		switch {
		case m == nil:
			problems = append(problems, r.ID+" is not in EARS form: write \"When …, the "+s.Service+" service shall …\" (or While, Where, If … then, or \"The "+s.Service+" service shall …\")")
		case !strings.Contains(strings.ToLower(m[1]), service):
			problems = append(problems, r.ID+" names \""+m[1]+"\" as its subject; a "+s.Service+" spec states what "+s.Service+" shall do")
		}
		if w := weakRe.FindString(r.Text); w != "" {
			problems = append(problems, r.ID+" uses \""+w+"\"; a requirement says shall, without hedging")
		}
	}
	for _, c := range s.Criteria {
		then := strings.TrimSpace(c.Then)
		switch {
		case vagueRe.MatchString(then):
			problems = append(problems, c.ID+": \"then "+then+"\" is not observable; name the status, value, event or record a test checks")
		case !observableRe.MatchString(then):
			problems = append(problems, c.ID+": \"then "+then+"\" names nothing a test can observe (a status code, a returned or stored value, an event, an error code)")
		}
		for _, m := range bareNumberRe.FindAllStringSubmatch(c.Given+" "+c.When+" "+c.Then, -1) {
			if m[2] == "" {
				problems = append(problems, c.ID+": \""+m[0]+"\" has no unit")
			}
		}
	}
	return problems
}

// Check is Lint plus every link (FR-SPEC-003, FR-DEC-004): the spec is for this change, cites
// PRD IDs and decisions that exist, each criterion covers a requirement of this spec, each
// requirement has a criterion, and no REQ or AC number repeats within the change.
func Check(s *Spec, ctx Context) []string {
	problems := Lint(s)
	if s.Change != ctx.Change {
		problems = append(problems, fmt.Sprintf("the spec is for %s, not %s", s.Change, ctx.Change))
	}
	reqs := map[string]bool{}
	covered := map[string]bool{}
	for _, r := range s.Requirements {
		reqs[r.ID] = true
		for _, p := range r.PRD {
			if !ctx.PRD(p) {
				problems = append(problems, r.ID+" cites "+p+", which the current PRD index does not have")
			}
		}
		for _, d := range r.Decisions {
			if !ctx.Decision(d) {
				problems = append(problems, r.ID+" cites decision "+d+", which decisions.yaml does not have")
			}
		}
	}
	for _, c := range s.Criteria {
		for _, r := range c.Req {
			if !reqs[r] {
				problems = append(problems, c.ID+" covers "+r+", which this spec does not define")
			}
			covered[r] = true
		}
	}
	for _, r := range s.Requirements {
		if !covered[r.ID] {
			problems = append(problems, r.ID+" has no acceptance criterion")
		}
	}
	return append(problems, duplicates(s, ctx.Others)...)
}

// duplicates finds REQ and AC numbers used twice in this spec or already used by another
// service's spec of the change.
func duplicates(s *Spec, others []*Spec) []string {
	var problems []string
	seen := map[string]string{}
	for _, o := range others {
		if o.Service == s.Service {
			continue
		}
		for _, id := range ids(o) {
			seen[id] = o.Service
		}
	}
	mine := map[string]bool{}
	for _, id := range ids(s) {
		switch {
		case mine[id]:
			problems = append(problems, id+" is used twice in this spec")
		case seen[id] != "":
			problems = append(problems, id+" is already used by the "+seen[id]+" spec; REQ and AC numbers are unique per change")
		}
		mine[id] = true
	}
	return problems
}

func ids(s *Spec) []string {
	out := make([]string, 0, len(s.Requirements)+len(s.Criteria))
	for _, r := range s.Requirements {
		out = append(out, r.ID)
	}
	for _, c := range s.Criteria {
		out = append(out, c.ID)
	}
	return out
}

// Render writes spec/<service>.md in doc 05's layout, then the block.
func Render(s *Spec, block []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Spec: %s · %s\n\n## Requirements\n", s.Service, s.Change)
	for _, r := range s.Requirements {
		fmt.Fprintf(&b, "- %s (%s): %s", r.ID, strings.Join(r.PRD, ", "), r.Text)
		if len(r.Decisions) > 0 {
			fmt.Fprintf(&b, " [Decision %s]", strings.Join(r.Decisions, ", "))
		}
		b.WriteString("\n")
	}
	section := func(title string, lines []string) {
		if len(lines) == 0 {
			lines = []string{"none"}
		}
		b.WriteString("\n## " + title + "\n")
		for _, l := range lines {
			b.WriteString("- " + l + "\n")
		}
	}
	var lines []string
	for _, i := range s.Interfaces {
		lines = append(lines, fmt.Sprintf("%s %s (%s): %s", i.Kind, i.Name, i.Change, i.Description))
	}
	section("Interfaces changed", lines)
	lines = nil
	for _, d := range s.DataChanges {
		lines = append(lines, fmt.Sprintf("%s: %s, %s", d.Table, d.Change, d.Description))
	}
	section("Data changes", lines)
	lines = nil
	for _, c := range s.Criteria {
		lines = append(lines, fmt.Sprintf("%s (%s): Given %s, when %s, then %s.", c.ID, strings.Join(c.Req, ", "), c.Given, c.When, c.Then))
	}
	section("Acceptance criteria", lines)
	section("Out of scope", s.OutOfScope)
	if len(s.DesignNotes) > 0 {
		section("Design notes", s.DesignNotes)
	}
	if len(s.Assumptions) > 0 {
		lines = nil
		for _, a := range s.Assumptions {
			lines = append(lines, a.ID+": "+a.Text)
		}
		section("Assumptions", lines)
	}
	b.WriteString("\n## Spec block (athel/spec@1, checked by the athel CLI)\n\n" + blockStart)
	b.Write(bytes.TrimRight(block, "\n"))
	b.WriteString(blockEnd)
	return b.Bytes()
}

const blockStart, blockEnd = "```yaml\n", "\n```\n"

// Extract reads the block back from spec/<service>.md.
func Extract(reg *schema.Registry, md []byte) (*Spec, error) {
	start := bytes.LastIndex(md, []byte(blockStart))
	if start < 0 {
		return nil, aerr.New(aerr.StateSchema, "the spec file has no yaml block")
	}
	body := md[start+len(blockStart):]
	end := bytes.Index(body, []byte(blockEnd))
	if end < 0 {
		return nil, aerr.New(aerr.StateSchema, "the spec file has an unterminated yaml block")
	}
	return Decode(reg, body[:end+1])
}
