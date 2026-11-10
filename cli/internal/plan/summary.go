package plan

import (
	"encoding/json"
	"slices"
	"strings"
)

// Estimates per task size, the rough figures doc 05's one-screen summary shows before approval.
// They are a planning aid, not a budget: the budget gate (doc 02) is what stops spending.
var estimates = map[string]struct{ Hours, USD float64 }{
	"S": {0.5, 2}, "M": {1.5, 6}, "L": {3, 12},
}

// RepoTasks is one repo's tasks in the order they can run.
type RepoTasks struct {
	Repo  string   `json:"repo"`
	Tasks []string `json:"tasks"`
}

// Summary is doc 05's one-screen plan summary (§Approval).
type Summary struct {
	Change      string      `json:"change"`
	Repos       []RepoTasks `json:"repos"`
	Gates       []string    `json:"gates"`
	EnvActions  []string    `json:"env_actions"`
	Risk        string      `json:"risk"`
	Assumptions []string    `json:"open_assumptions"`
	Hours       float64     `json:"estimated_hours"`
	USD         float64     `json:"estimated_cost_usd"`
}

// Summarise orders the tasks (dependencies first, then by ID, so the same plan always reads the
// same) and adds up the gates, env actions and estimates.
func Summarise(p *Plan, risk string, assumptions []string) Summary {
	s := Summary{Change: p.Change, Repos: []RepoTasks{}, Gates: []string{}, EnvActions: []string{}, Risk: risk, Assumptions: assumptions}
	if s.Assumptions == nil {
		s.Assumptions = []string{}
	}
	byRepo := map[string]*RepoTasks{}
	var repos []string // in the order their first task runs
	for _, t := range p.Order() {
		rt := byRepo[t.Repo]
		if rt == nil {
			rt = &RepoTasks{Repo: t.Repo}
			byRepo[t.Repo] = rt
			repos = append(repos, t.Repo)
		}
		rt.Tasks = append(rt.Tasks, t.ID+" "+t.Title)
		for _, g := range t.Gates {
			if !slices.Contains(s.Gates, g) {
				s.Gates = append(s.Gates, g)
			}
		}
		for _, a := range t.EnvActions {
			s.EnvActions = append(s.EnvActions, t.ID+": "+a)
		}
		e := estimates[t.Size]
		s.Hours += e.Hours
		s.USD += e.USD
	}
	for _, r := range repos {
		s.Repos = append(s.Repos, *byRepo[r])
	}
	return s
}

// Order is the tasks in a runnable order: a task comes after all it depends on, and among the
// ready tasks the lowest ID goes first.
func (p *Plan) Order() []Task {
	done := map[string]bool{}
	var out []Task
	for len(out) < len(p.Tasks) {
		var ready []Task
		for _, t := range p.Tasks {
			if !done[t.ID] && !slices.ContainsFunc(t.DependsOn, func(d string) bool { return !done[d] }) {
				ready = append(ready, t)
			}
		}
		slices.SortFunc(ready, func(a, b Task) int { return strings.Compare(a.ID, b.ID) })
		next := ready[0] // the graph is acyclic (Decode), so some task is always ready
		done[next.ID] = true
		out = append(out, next)
	}
	return out
}

// TaskDiff is how a plan changed since the approved one.
type TaskDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// Compare lists tasks added, removed and changed between two plans, so a small change after
// approval is re-presented as a diff, not a whole new plan (doc 05 §Approval).
func Compare(old, cur *Plan) TaskDiff {
	d := TaskDiff{Added: []string{}, Removed: []string{}, Changed: []string{}}
	index := func(p *Plan) map[string]Task {
		m := map[string]Task{}
		for _, t := range p.Tasks {
			m[t.ID] = t
		}
		return m
	}
	o, c := index(old), index(cur)
	for _, t := range cur.Tasks {
		prev, ok := o[t.ID]
		switch {
		case !ok:
			d.Added = append(d.Added, t.ID)
		case !sameTask(prev, t):
			d.Changed = append(d.Changed, t.ID)
		}
	}
	for _, t := range old.Tasks {
		if _, ok := c[t.ID]; !ok {
			d.Removed = append(d.Removed, t.ID)
		}
	}
	return d
}

func sameTask(a, b Task) bool {
	x, _ := json.Marshal(a) // strings, booleans and slices only
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
