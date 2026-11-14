// Package findings is doc 07's side findings (FR-FIND-001 to 003, FR-BUILD-013): problems any
// agent notices outside the task's scope, recorded and never fixed inside the change, ranked so
// the cheap and safe ones are easy to accept, and presented once, at PR time, for the developer
// to choose: a separate fix PR, a ticket, or ignore.
package findings

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// SchemaID is findings.yaml's format.
const SchemaID = "athel/finding@1"

// Dispositions the developer chooses (FR-FIND-003).
const (
	FixPR  = "fix_pr"
	Ticket = "ticket"
	Ignore = "ignore"
)

var (
	categories = []string{"bug", "security", "performance", "reliability", "dead-code", "dependency", "test-gap", "docs"}
	severities = []string{"critical", "high", "medium", "low"} // highest first
	efforts    = []string{"S", "M", "L"}                       // S < 30 lines, M < 150, L larger
	risks      = []string{"low", "medium", "high"}
)

// Where says where a finding was noticed.
type Where struct {
	Step string `yaml:"step" json:"step"`
	Task string `yaml:"task,omitempty" json:"task,omitempty"`
}

// Draft is a finding as an agent records it; the CLI gives it its ID.
type Draft struct {
	FoundBy      string `yaml:"found_by"`
	FoundIn      Where  `yaml:"found_in"`
	Location     string `yaml:"location"`
	Category     string `yaml:"category"`
	Severity     string `yaml:"severity"`
	Effort       string `yaml:"effort"`
	FixRisk      string `yaml:"fix_risk"`
	Evidence     string `yaml:"evidence"`
	SuggestedFix string `yaml:"suggested_fix"`
}

// Finding is doc 07's finding record.
type Finding struct {
	ID           string  `yaml:"id" json:"id"`
	FoundBy      string  `yaml:"found_by" json:"found_by"`
	FoundIn      Where   `yaml:"found_in" json:"found_in"`
	Location     string  `yaml:"location" json:"location"`
	Category     string  `yaml:"category" json:"category"`
	Severity     string  `yaml:"severity" json:"severity"`
	Effort       string  `yaml:"effort" json:"effort"`
	FixRisk      string  `yaml:"fix_risk" json:"fix_risk"`
	Evidence     string  `yaml:"evidence" json:"evidence"`
	SuggestedFix string  `yaml:"suggested_fix" json:"suggested_fix"`
	Disposition  *string `yaml:"disposition" json:"disposition"`
	DecidedBy    string  `yaml:"decided_by,omitempty" json:"decided_by,omitempty"`
	DecidedAt    string  `yaml:"decided_at,omitempty" json:"decided_at,omitempty"`
	Child        string  `yaml:"child,omitempty" json:"child,omitempty"` // the fix's change, FR-FIND-004
	QuickWin     bool    `yaml:"-" json:"quick_win"`
}

// File is findings.yaml.
type File struct {
	Schema   string    `yaml:"schema" json:"schema"`
	Findings []Finding `yaml:"findings" json:"findings"`
}

func invalid(msg string) error {
	return aerr.New(aerr.StateInvalid, "finding rejected: "+msg).WithNextAction("fix the finding and add it again")
}

// Add checks a draft (FR-FIND-001: location, evidence, category, severity, effort, fix risk and
// suggested fix) and appends it with the next SF-n.
func (f *File) Add(d Draft) (Finding, error) {
	switch {
	case !slices.Contains(categories, d.Category):
		return Finding{}, invalid("category is one of " + strings.Join(categories, ", "))
	case !slices.Contains(severities, d.Severity):
		return Finding{}, invalid("severity is one of " + strings.Join(severities, ", "))
	case !slices.Contains(efforts, d.Effort):
		return Finding{}, invalid("effort is S (under 30 lines), M (under 150) or L")
	case !slices.Contains(risks, d.FixRisk):
		return Finding{}, invalid("fix_risk is low, medium or high")
	case d.FoundBy == "" || d.Location == "" || d.Evidence == "" || d.SuggestedFix == "":
		return Finding{}, invalid("a finding names who found it, where, its evidence and a suggested fix")
	}
	x := Finding{ID: "SF-" + strconv.Itoa(len(f.Findings)+1), FoundBy: d.FoundBy, FoundIn: d.FoundIn, Location: d.Location,
		Category: d.Category, Severity: d.Severity, Effort: d.Effort, FixRisk: d.FixRisk, Evidence: d.Evidence, SuggestedFix: d.SuggestedFix}
	f.Findings = append(f.Findings, x)
	return x, nil
}

// quickWin is doc 07's "effort S and fix_risk low".
func quickWin(x Finding) bool { return x.Effort == "S" && x.FixRisk == "low" }

// Ranked is the findings in doc 07's order: security first, then severity high to low, then
// small effort and low fix risk first; ties keep their order. Quick wins are marked.
func (f *File) Ranked() []Finding {
	out := slices.Clone(f.Findings)
	for i := range out {
		out[i].QuickWin = quickWin(out[i])
	}
	key := func(x Finding) []int {
		sec := 1
		if x.Category == "security" {
			sec = 0
		}
		return []int{sec, slices.Index(severities, x.Severity), slices.Index(efforts, x.Effort), slices.Index(risks, x.FixRisk)}
	}
	slices.SortStableFunc(out, func(a, b Finding) int { return slices.Compare(key(a), key(b)) })
	return out
}

// Summary is doc 07's one-line presentation.
func (f *File) Summary() string {
	if len(f.Findings) == 0 {
		return "No issues found outside this change."
	}
	var wins []string
	for _, x := range f.Ranked() {
		if x.QuickWin {
			wins = append(wins, x.ID)
		}
	}
	s := fmt.Sprintf("Found %d issue%s outside this change.", len(f.Findings), plural(len(f.Findings)))
	if len(wins) > 0 {
		s += fmt.Sprintf(" %d quick win%s: %s.", len(wins), plural(len(wins)), strings.Join(wins, ", "))
	}
	return s + " Fix them as separate PRs, file tickets, or ignore?"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Triage records the developer's dispositions (FR-FIND-003).
func (f *File) Triage(choices map[string]string, by, at string) error {
	if len(choices) == 0 {
		return invalid("no finding chosen")
	}
	for id, d := range choices {
		if d != FixPR && d != Ticket && d != Ignore {
			return invalid(id + ": a disposition is fix_pr, ticket or ignore")
		}
		if !slices.ContainsFunc(f.Findings, func(x Finding) bool { return x.ID == id }) {
			return aerr.New(aerr.StateNotFound, "no side finding "+id)
		}
	}
	for i := range f.Findings {
		if d, ok := choices[f.Findings[i].ID]; ok {
			f.Findings[i].Disposition, f.Findings[i].DecidedBy, f.Findings[i].DecidedAt = &d, by, at
		}
	}
	return nil
}

// Undecided lists the findings with no disposition yet.
func (f *File) Undecided() []string {
	var out []string
	for _, x := range f.Findings {
		if x.Disposition == nil {
			out = append(out, x.ID)
		}
	}
	return out
}
