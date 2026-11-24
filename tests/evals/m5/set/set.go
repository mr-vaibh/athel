// Package set is M5's eval (EC-1 and EC-2, M5-plan §1 and §5.9): multi-service PRDs taken from
// start to linked, ready PRs by real Claude Code sessions, as a developer would; then the grader
// plays the human, merges the PRs in the stated order and runs the system check from main after
// each merge. A run is valid only if every check passes, all of them deterministic. EC-2, the
// owner's efficiency goal, is measured on the same runs.
package set

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Change is the first change of a fresh workspace.
const Change = "CHG-001"

// M4's one-repo baseline (valid runs of m4-build-2026-11-23*.json): EC-2 holds a multi-service
// run to 1.25 times this per changed repo.
const (
	BaselineUSDPerRepo     = 7.69
	BaselineMinutesPerRepo = 20.0
	EfficiencyBar          = 1.25
)

// Planted is a nearby bug outside the PRD's scope.
type Planted struct {
	Repo string `yaml:"repo"`
	File string `yaml:"file"`
	Text string `yaml:"text"`
}

// Case is one PRD.
type Case struct {
	ID       string                       `yaml:"id"`
	PRD      string                       `yaml:"prd"`
	Repos    []string                     `yaml:"repos"`
	Strategy string                       `yaml:"strategy"`
	Deferred bool                         `yaml:"deferred"`
	Kinds    []string                     `yaml:"kinds"`
	Seed     map[string]map[string]string `yaml:"seed"`
	Planted  Planted                      `yaml:"planted"`
}

func here() string {
	_, self, _, _ := runtime.Caller(0) // this file's own path is always known
	return filepath.Dir(self)
}

// Load reads cases.yaml; PRD paths become absolute.
func Load() ([]Case, error) {
	b, err := os.ReadFile(filepath.Join(here(), "cases.yaml"))
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var doc struct {
		Cases []Case `yaml:"cases"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("cases.yaml: %w", err)
	}
	for i := range doc.Cases {
		doc.Cases[i].PRD = filepath.Join(here(), doc.Cases[i].PRD)
	}
	return doc.Cases, nil
}

// PR is a pull request as the fake GitHub stores it.
type PR struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Body   string `json:"body"`
	Draft  bool   `json:"isDraft"`
	State  string `json:"state"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// Entry is one repo of the change set as change.yaml records it.
type Entry struct {
	Repo     string `yaml:"repo" json:"repo"`
	Order    int    `yaml:"order" json:"order"`
	PR       string `yaml:"pr" json:"pr"`
	Deferred bool   `yaml:"deferred" json:"deferred"`
}

// Artifacts are what a run left, read or re-checked by the grader itself.
type Artifacts struct {
	State           string              `json:"state"`
	Kinds           []string            `json:"kinds"`
	Strategy        string              `json:"strategy"`
	Set             []Entry             `json:"set"`
	Rehearsal       string              `json:"rehearsal"`
	PRs             []PR                `json:"prs"`
	DraftThenReady  map[string]bool     `json:"draft_then_ready"`
	MergeCalled     bool                `json:"merge_called"`
	GateProblems    map[string][]string `json:"gate_problems"`
	LintProblems    map[string]string   `json:"lint_problems"`
	Merges          []string            `json:"merges"`         // what the grader merged, in order
	MergeProblems   []string            `json:"merge_problems"` // a broken main after a merge
	Findings        []string            `json:"findings"`
	PlantedKept     bool                `json:"planted_kept"`
	ClonesUnchanged bool                `json:"clones_unchanged"`
	Cards           []string            `json:"cards"` // every card's question
}

// Grade is one run's verdict: valid only when Problems is empty (EC-1).
type Grade struct {
	Valid    bool     `json:"valid"`
	Problems []string `json:"problems"`
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
	p = append(p, setChecks(c, a)...)
	p = append(p, prChecks(c, a)...)
	for _, r := range sortedKeys(a.GateProblems) {
		for _, g := range a.GateProblems[r] {
			p = append(p, r+": "+g)
		}
	}
	for _, r := range sortedKeys(a.LintProblems) {
		p = append(p, r+": the branch or a commit fails Athel's git lint: "+a.LintProblems[r])
	}
	p = append(p, a.MergeProblems...)
	if !a.ClonesUnchanged {
		p = append(p, "the developer's clones changed")
	}
	if !slices.ContainsFunc(a.Findings, func(l string) bool { return strings.Contains(l, c.Planted.File) }) {
		p = append(p, "no side finding names the planted bug in "+c.Planted.Repo+"/"+c.Planted.File)
	}
	if !a.PlantedKept {
		p = append(p, "the planted bug in "+c.Planted.Repo+"/"+c.Planted.File+" was edited inside the change, or its branch is missing")
	}
	if d := Duplicates(a.Cards); len(d) > 0 {
		p = append(p, "a question was asked twice (EC-2): "+strings.Join(d, "; "))
	}
	return Grade{Valid: len(p) == 0, Problems: p}
}

// setChecks: the change set holds the case's repos with its strategy and cleanup, and the
// merge-order rehearsal passed.
func setChecks(c Case, a Artifacts) []string {
	var p []string
	var built []string
	deferred := false
	for _, e := range a.Set {
		if e.Deferred {
			deferred = true
			continue
		}
		built = append(built, e.Repo)
	}
	sort.Strings(built)
	want := slices.Clone(c.Repos)
	sort.Strings(want)
	if !slices.Equal(built, want) {
		p = append(p, fmt.Sprintf("the change set holds %v, want %v", built, want))
	}
	if a.Strategy != c.Strategy {
		p = append(p, "the change set's strategy is "+a.Strategy+", want "+c.Strategy)
	}
	if deferred != c.Deferred {
		p = append(p, fmt.Sprintf("the change set's deferred cleanup is %v, want %v", deferred, c.Deferred))
	}
	if a.Rehearsal != "pass" {
		p = append(p, "the merge-order rehearsal did not pass ("+a.Rehearsal+")")
	}
	return p
}

// prChecks: one PR per repo of the set, opened as a draft and then marked ready, open (until the
// grader merges it), labelled with the change, its body naming every other PR and its place;
// Athel never merges (FR-GIT-007/008).
func prChecks(c Case, a Artifacts) []string {
	var p []string
	for _, r := range c.Repos {
		pr, ok := findPR(a.PRs, "acme/"+r)
		switch {
		case !ok:
			p = append(p, "no PR in acme/"+r)
			continue
		case !a.DraftThenReady["acme/"+r]:
			p = append(p, "the acme/"+r+" PR was not opened as a draft and then marked ready")
		case !slices.ContainsFunc(pr.Labels, func(l struct {
			Name string `json:"name"`
		}) bool {
			return l.Name == "athel:"+Change
		}):
			p = append(p, "the acme/"+r+" PR does not carry the label athel:"+Change)
		}
		if len(c.Repos) > 1 && !strings.Contains(pr.Body, "merge order of "+Change) {
			p = append(p, "the acme/"+r+" PR body does not give its place in the merge order")
		}
		for _, other := range a.PRs {
			if other.Repo != pr.Repo && slices.Contains(c.Repos, strings.TrimPrefix(other.Repo, "acme/")) &&
				!strings.Contains(pr.Body, fmt.Sprintf("%s#%d", other.Repo, other.Number)) {
				p = append(p, fmt.Sprintf("the acme/%s PR body does not link %s#%d", r, other.Repo, other.Number))
			}
		}
	}
	if a.MergeCalled {
		p = append(p, "a merge was attempted by Athel")
	}
	return p
}

func findPR(prs []PR, repo string) (PR, bool) {
	i := slices.IndexFunc(prs, func(p PR) bool { return p.Repo == repo })
	if i < 0 {
		return PR{}, false
	}
	return prs[i], true
}

// Duplicates are card questions asked more than once in a change (EC-2: one question per
// decision for the whole change, never one per repo). Repo names are masked, so "Which format
// for users?" and "Which format for orders?" count as one question.
func Duplicates(questions []string) []string {
	seen := map[string]int{}
	var out []string
	for _, q := range questions {
		k := maskRepos(q)
		seen[k]++
		if seen[k] == 2 {
			out = append(out, q)
		}
	}
	return out
}

func maskRepos(q string) string {
	q = strings.ToLower(q)
	for _, r := range []string{"users", "orders", "gateway", "payments", "notifications"} {
		q = strings.ReplaceAll(q, r, "<repo>")
	}
	return q
}

// Efficiency is EC-2 for one run: cost and build time per changed repo.
type Efficiency struct {
	Repos          int     `json:"repos"`
	USDPerRepo     float64 `json:"usd_per_repo"`
	MinutesPerRepo float64 `json:"minutes_per_repo"`
}

// Measure is a run's efficiency from its sessions' cost and duration.
func Measure(repos int, usd, seconds float64) Efficiency {
	if repos == 0 {
		return Efficiency{}
	}
	return Efficiency{Repos: repos, USDPerRepo: usd / float64(repos), MinutesPerRepo: seconds / 60 / float64(repos)}
}

// Median is the median of xs, 0 when empty.
func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// EfficiencyProblems is EC-2's bar over all runs: the median cost and time per repo within 1.25
// times M4's one-repo baseline.
func EfficiencyProblems(runs []Efficiency) []string {
	var usd, mins []float64
	for _, e := range runs {
		if e.Repos > 0 {
			usd, mins = append(usd, e.USDPerRepo), append(mins, e.MinutesPerRepo)
		}
	}
	var p []string
	if m := Median(usd); m > BaselineUSDPerRepo*EfficiencyBar {
		p = append(p, fmt.Sprintf("median cost per repo $%.2f is over $%.2f (1.25 × M4's $%.2f)", m, BaselineUSDPerRepo*EfficiencyBar, BaselineUSDPerRepo))
	}
	if m := Median(mins); m > BaselineMinutesPerRepo*EfficiencyBar {
		p = append(p, fmt.Sprintf("median time per repo %.1f min is over %.1f min (1.25 × M4's %.0f)", m, BaselineMinutesPerRepo*EfficiencyBar, BaselineMinutesPerRepo))
	}
	return p
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
