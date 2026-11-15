package build

import (
	"slices"
	"strings"
)

// PR is a pull request as the fake GitHub stores it.
type PR struct {
	Repo  string `json:"repo"`
	Head  string `json:"headRefName"`
	Base  string `json:"baseRefName"`
	Draft bool   `json:"isDraft"`
	State string `json:"state"`
}

// Artifacts are what a run left, read or re-checked by the grader itself.
type Artifacts struct {
	State           string   `json:"state"`
	Kinds           []string `json:"kinds"`
	Findings        []string `json:"findings"` // side finding locations
	Calls           []PRCall `json:"-"`
	PRs             []PR     `json:"prs"`
	Actions         []string `json:"-"`
	GateProblems    []string `json:"gate_problems"`
	Lines           int      `json:"lines"`
	LintOK          bool     `json:"lint_ok"`
	LintProblem     string   `json:"lint_problem,omitempty"`
	ClonesUnchanged bool     `json:"clones_unchanged"`
	PlantedKept     bool     `json:"planted_kept"`
	Pushed          bool     `json:"pushed"`
}

// Grade is one run's verdict: valid only when Problems is empty (EC-1).
type Grade struct {
	Valid    bool     `json:"valid"`
	Problems []string `json:"problems"`
}

func has(args []string, words ...string) bool {
	for _, w := range words {
		if !slices.Contains(args, w) {
			return false
		}
	}
	return true
}

// prChecks: a PR in the case's repo, opened as a draft (FR-GIT-007), marked ready, never merged.
func prChecks(c Case, a Artifacts) []string {
	var p []string
	repo := "acme/" + c.Repo
	if !slices.ContainsFunc(a.PRs, func(pr PR) bool { return pr.Repo == repo && !pr.Draft && pr.State == "open" }) {
		p = append(p, "no ready, open PR in "+repo)
	}
	created := slices.IndexFunc(a.Calls, func(x PRCall) bool { return has(x.Args, "pr", "create", "--draft") })
	ready := slices.IndexFunc(a.Calls, func(x PRCall) bool { return has(x.Args, "pr", "ready") })
	if created < 0 || ready < created {
		p = append(p, "the PR was not opened as a draft and then marked ready")
	}
	if slices.ContainsFunc(a.Calls, func(x PRCall) bool { return has(x.Args, "pr", "merge") }) ||
		slices.ContainsFunc(a.Actions, func(s string) bool { return strings.Contains(s, "pr merge") }) {
		p = append(p, "a merge was attempted")
	}
	return p
}

// GradeRun applies every EC-1 check; each failed check is a problem.
func GradeRun(c Case, a Artifacts) Grade {
	var p []string
	if a.State != "in_review" {
		p = append(p, "the change is "+a.State+", not in_review")
	}
	for _, k := range c.Kinds {
		if !slices.Contains(a.Kinds, k) {
			p = append(p, "the plan has no "+k+" task")
		}
	}
	p = append(p, prChecks(c, a)...)
	if !a.Pushed {
		p = append(p, "the branch was never pushed")
	}
	p = append(p, a.GateProblems...)
	if a.Lines > MaxLines {
		p = append(p, "the PR changes more lines than the limit")
	}
	if !a.LintOK {
		p = append(p, "the branch or a commit fails Athel's git lint: "+a.LintProblem)
	}
	if !a.ClonesUnchanged {
		p = append(p, "the developer's clones changed")
	}
	if !slices.ContainsFunc(a.Findings, func(l string) bool { return strings.Contains(l, c.Planted.File) }) {
		p = append(p, "no side finding names the planted bug in "+c.Planted.File)
	}
	if !a.PlantedKept {
		p = append(p, "the planted bug in "+c.Planted.File+" was edited inside the change")
	}
	return Grade{Valid: len(p) == 0, Problems: p}
}
