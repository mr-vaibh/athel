// Package review is doc 06's iterative review protocol (FR-BUILD-007/008/010): review rounds
// whose findings are tracked until a verified fix or a human waiver closes them. Round 1 is a
// full review; each later round carries every unresolved finding forward and judges it again,
// never restarting from zero. Only the reviewer verifies; only the developer fixes or disputes.
package review

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/quality"
)

// SchemaID is a round file's format.
const SchemaID = "athel/review@1"

// MaxRounds is doc 06's round limit (FR-BUILD-010); the next round is a decision card.
const MaxRounds = 5

// RoundCeiling bounds the rounds a developer can allow, 5 at a time, past MaxRounds.
const RoundCeiling = 50

// Severities (doc 06 table).
const (
	Blocker = "blocker"
	Major   = "major"
	Minor   = "minor"
	Nit     = "nit"
)

// Finding statuses (doc 06 finding format).
const (
	Open     = "open"
	Fixed    = "fixed"
	Verified = "verified"
	Waived   = "waived"
	Disputed = "disputed"
)

// Verdicts.
const (
	Approved         = "approved"
	ChangesRequested = "changes_requested"
)

var (
	severities = []string{Blocker, Major, Minor, Nit}
	sources    = []string{"reviewer", "security", "qa", "gate"}
)

// Step is one status change of a finding.
type Step struct {
	Round  int    `yaml:"round" json:"round"`
	Status string `yaml:"status" json:"status"`
	Note   string `yaml:"note,omitempty" json:"note,omitempty"`
}

// Finding is doc 06's finding record.
type Finding struct {
	ID       string `yaml:"id" json:"id"`
	Severity string `yaml:"severity" json:"severity"`
	Source   string `yaml:"source" json:"source"`
	Location string `yaml:"location" json:"location"`
	Rule     string `yaml:"rule" json:"rule"`
	Finding  string `yaml:"finding" json:"finding"`
	Required string `yaml:"required" json:"required"`
	Status   string `yaml:"status" json:"status"`
	FixedIn  string `yaml:"fixed_in,omitempty" json:"fixed_in,omitempty"`
	Dispute  string `yaml:"dispute,omitempty" json:"dispute,omitempty"`
	From     string `yaml:"from,omitempty" json:"from,omitempty"` // the QA or security report finding
	History  []Step `yaml:"history" json:"history"`
}

// Round is one review round (reviews/<task>/round-N.yaml).
type Round struct {
	Schema   string    `yaml:"schema" json:"schema"`
	Round    int       `yaml:"round" json:"round"`
	Head     string    `yaml:"head" json:"head"` // the commit reviewed
	Verdict  string    `yaml:"verdict" json:"verdict"`
	Findings []Finding `yaml:"findings" json:"findings"`
}

// Draft is a new finding as the reviewer writes it; the CLI gives it its ID and status.
type Draft struct {
	Severity string `yaml:"severity"`
	Source   string `yaml:"source"`
	Location string `yaml:"location"`
	Rule     string `yaml:"rule"`
	Finding  string `yaml:"finding"`
	Required string `yaml:"required"`
}

// Resolution is the reviewer's judgement of an earlier finding: verified (fixed, or the
// dispute accepted) or still open.
type Resolution struct {
	ID     string `yaml:"id"`
	Status string `yaml:"status"`
	Note   string `yaml:"note"`
}

// Submission is what the reviewer sends for a round.
type Submission struct {
	Verdict  string       `yaml:"verdict"`
	Previous []Resolution `yaml:"previous"`
	New      []Draft      `yaml:"new"`
}

func invalid(msg string) error {
	return aerr.New(aerr.StateInvalid, "review rejected: "+msg).
		WithNextAction("fix the review and save it again; athel review template shows the format")
}

// Next makes round N+1 from round N (nil for the first round) and the reviewer's submission.
// limit is the task's round limit: MaxRounds, or more once the developer allowed more rounds.
func Next(prev *Round, s Submission, head string, limit int) (*Round, error) {
	n := 1
	if prev != nil {
		n = prev.Round + 1
	}
	if n > limit {
		return nil, aerr.New(aerr.BudgetRounds, fmt.Sprintf("the task has had %d review rounds, its limit", limit)).
			WithNextAction("raise a decision card: continue with a human, or stop the task (FR-BUILD-010)")
	}
	if s.Verdict != Approved && s.Verdict != ChangesRequested {
		return nil, invalid("verdict is approved or changes_requested")
	}
	r := &Round{Schema: SchemaID, Round: n, Head: head, Verdict: s.Verdict, Findings: []Finding{}}
	if prev != nil {
		if err := carry(prev, s.Previous, r); err != nil {
			return nil, err
		}
	} else if len(s.Previous) > 0 {
		return nil, invalid("the first round has no earlier findings to judge")
	}
	for i, d := range s.New {
		if err := checkDraft(d); err != nil {
			return nil, err
		}
		r.Findings = append(r.Findings, Finding{ID: fmt.Sprintf("R%d-%02d", n, i+1), Severity: d.Severity, Source: d.Source,
			Location: d.Location, Rule: d.Rule, Finding: d.Finding, Required: d.Required, Status: Open, History: []Step{{Round: n, Status: Open}}})
	}
	return r, nil
}

func checkDraft(d Draft) error {
	switch {
	case !slices.Contains(severities, d.Severity):
		return invalid("severity is one of " + strings.Join(severities, ", "))
	case !slices.Contains(sources, d.Source):
		return invalid("source is one of " + strings.Join(sources, ", "))
	case d.Location == "" || d.Rule == "" || d.Finding == "" || d.Required == "":
		return invalid("every finding has a location, a rule, the finding and the required fix or question (FR-BUILD-007)")
	case !quality.Known(d.Rule):
		return invalid(d.Rule + " is not a rule of the code quality standard; athel review rules lists them")
	}
	return nil
}

// carry copies round prev's findings into r: resolved ones as they are, and every unresolved one
// with the reviewer's new judgement, which the submission must give (FR-BUILD-008).
func carry(prev *Round, res []Resolution, r *Round) error {
	byID := map[string]Resolution{}
	for _, x := range res {
		byID[x.ID] = x
	}
	var missing []string
	for _, f := range prev.Findings {
		f.History = slices.Clone(f.History)
		if f.Status == Verified || f.Status == Waived {
			r.Findings = append(r.Findings, f)
			continue
		}
		x, ok := byID[f.ID]
		delete(byID, f.ID)
		switch {
		case !ok:
			missing = append(missing, f.ID)
			continue
		case x.Status == Verified && f.Status != Fixed && f.Status != Disputed:
			return invalid(f.ID + " is " + f.Status + ", not fixed; only a fix or an accepted dispute is verified")
		case x.Status != Verified && x.Status != Open:
			return invalid(f.ID + ": the reviewer marks an earlier finding verified or open; only a human waives")
		}
		f.Status = x.Status
		f.History = append(f.History, Step{Round: r.Round, Status: x.Status, Note: x.Note})
		r.Findings = append(r.Findings, f)
	}
	switch {
	case len(missing) > 0:
		return invalid("round " + fmt.Sprint(r.Round) + " does not judge " + strings.Join(missing, ", ") + " from round " + fmt.Sprint(prev.Round))
	case len(byID) > 0:
		return invalid("no earlier finding to judge with that ID")
	}
	return nil
}

// Folded is a QA or security report finding as it joins a round.
type Folded struct {
	From, Source, Severity, Location, Rule, Finding, Required string
}

// Fold adds the report findings the round does not hold yet, as open findings numbered after
// the round's own. QA and security work in parallel with the reviewer, so a report may come
// before or after the round it joins; either way its findings are tracked like the reviewer's.
func (r *Round) Fold(fs []Folded) int {
	next := 1
	for _, f := range r.Findings {
		var n, i int
		if _, err := fmt.Sscanf(f.ID, "R%d-%d", &n, &i); err == nil && n == r.Round && i >= next {
			next = i + 1
		}
	}
	added := 0
	for _, x := range fs {
		if slices.ContainsFunc(r.Findings, func(f Finding) bool { return f.From == x.From }) {
			continue
		}
		r.Findings = append(r.Findings, Finding{ID: fmt.Sprintf("R%d-%02d", r.Round, next), Severity: x.Severity, Source: x.Source,
			Location: x.Location, Rule: x.Rule, Finding: x.Finding, Required: x.Required, Status: Open, From: x.From,
			History: []Step{{Round: r.Round, Status: Open, Note: "from " + x.From}}})
		next++
		added++
	}
	return added
}

// find returns a finding of the round.
func (r *Round) find(id string) (*Finding, error) {
	for i := range r.Findings {
		if r.Findings[i].ID == id {
			return &r.Findings[i], nil
		}
	}
	return nil, aerr.New(aerr.StateNotFound, "round "+fmt.Sprint(r.Round)+" has no finding "+id)
}

// Fix marks a finding fixed by a commit (the developer's action).
func (r *Round) Fix(id, commit string) error {
	f, err := r.find(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(commit) == "" {
		return invalid("a fix names its commit")
	}
	f.Status, f.FixedIn = Fixed, commit
	f.History = append(f.History, Step{Round: r.Round, Status: Fixed, Note: commit})
	return nil
}

// Dispute marks a finding disputed with the developer's written reason.
func (r *Round) Dispute(id, reason string) error {
	f, err := r.find(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return invalid("a dispute needs a written reason (doc 06 round rule 3)")
	}
	f.Status, f.Dispute = Disputed, reason
	f.History = append(f.History, Step{Round: r.Round, Status: Disputed, Note: reason})
	return nil
}

// Disputes counts the rounds in which a finding was disputed.
func (r *Round) Disputes(id string) int {
	f, err := r.find(id)
	if err != nil {
		return 0
	}
	rounds := map[int]bool{}
	for _, s := range f.History {
		if s.Status == Disputed {
			rounds[s.Round] = true
		}
	}
	return len(rounds)
}

// DisputedTwice lists the findings disputed in two rounds: each needs a decision card with both
// arguments (doc 06 round rule 3).
func (r *Round) DisputedTwice() []string {
	var out []string
	for _, f := range r.Findings {
		if f.Status == Disputed && r.Disputes(f.ID) >= 2 {
			out = append(out, f.ID)
		}
	}
	return out
}

// Blocking lists the blocker and major findings that keep the task from done: a blocker until
// verified, a major until verified or waived by a human (doc 06 severity table).
func (r *Round) Blocking() []string {
	var out []string
	for _, f := range r.Findings {
		closed := f.Status == Verified || (f.Severity == Major && f.Status == Waived)
		if (f.Severity == Blocker || f.Severity == Major) && !closed {
			out = append(out, f.ID)
		}
	}
	return out
}

// Done reports whether review ends for the task: the reviewer approved and nothing blocks
// (doc 06 round rule 4).
func (r *Round) Done() bool { return r.Verdict == Approved && len(r.Blocking()) == 0 }

func badLocation(msg string) error {
	return aerr.New(aerr.StateInvalid, "finding rejected: "+msg).WithNextAction("fix the finding's location and save again")
}

// CheckLocation checks a finding's location against the code it points at: path, path:line or
// path:from-to, with the file in the worktree and the lines inside it. lines returns the file's
// line count, or false when there is no such file. A wrong location sends the developer to the
// wrong place and cannot be verified later, so it is refused at once.
func CheckLocation(loc string, lines func(file string) (int, bool)) error {
	file, rng, ranged := strings.Cut(loc, ":")
	n, ok := lines(file)
	if file == "" || !ok {
		return badLocation(loc + ": no such file in the task's worktree; give the path from the repository root")
	}
	if !ranged {
		return nil
	}
	from, to := 0, 0
	a, b, span := strings.Cut(rng, "-")
	if !span {
		b = a
	}
	if x, err := strconv.Atoi(a); err == nil {
		from = x
	}
	if y, err := strconv.Atoi(b); err == nil {
		to = y
	}
	if from < 1 || to < from || to > n {
		return badLocation(fmt.Sprintf("%s: %s has %d lines; give line numbers of the file itself (Read it), not of the diff", loc, file, n))
	}
	return nil
}

// LocationFile is the file part of a finding's location ("path:42-58").
func LocationFile(loc string) string {
	file, _, _ := strings.Cut(loc, ":")
	return file
}

// Waive closes a major finding on a human's waiver (doc 06 severity table: a major is "fixed or
// waived by a human"; a blocker is only ever fixed, and minor and nit findings never block).
func (r *Round) Waive(id, reason string) error {
	f, err := r.find(id)
	if err != nil {
		return err
	}
	if f.Severity != Major {
		return aerr.New(aerr.StateInvalid, id+" is "+f.Severity+"; only a major finding is waived (a blocker must be fixed; minor and nit findings never block)")
	}
	f.Status = Waived
	f.History = append(f.History, Step{Round: r.Round, Status: Waived, Note: reason})
	return nil
}
