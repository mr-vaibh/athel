// Package changeset is the multi-repo change set of doc 04 §Multi-repo change sets
// (FR-GIT-008): one change, one branch and PR per repo, and a merge order that keeps every
// intermediate state of the system working. The CLI derives it from the approved plan, never
// the model (ADR 035): roles from task kinds, order from the cross-repo task dependencies.
package changeset

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// Strategies (doc 04 §Change set record and ordering rule 5).
const (
	Single         = "single"
	ExpandContract = "expand_contract"
	Coordinated    = "coordinated"
)

// Roles of a repo in the set.
const (
	Provider        = "provider"
	Consumer        = "consumer"
	ProviderCleanup = "provider_cleanup"
	Internal        = "internal"
)

// Invariant is doc 04's rule for every set.
const Invariant = "every merge in order leaves all services compatible"

// Entry is one repo's PR in the set, or the deferred cleanup of a repo.
type Entry struct {
	Repo      string   `yaml:"repo" json:"repo"`
	Role      string   `yaml:"role" json:"role"`
	Order     int      `yaml:"order" json:"order"`
	PR        *string  `yaml:"pr" json:"pr"`
	State     *string  `yaml:"state" json:"state"`
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
	Deferred  bool     `yaml:"deferred" json:"deferred"`
	Note      *string  `yaml:"note" json:"note"`
}

// Verify is the last merge-order rehearsal (ADR 036).
type Verify struct {
	At     string            `yaml:"at" json:"at"`
	Result string            `yaml:"result" json:"result"`
	Heads  map[string]string `yaml:"heads" json:"heads"`
}

// Set is change.yaml's change_set.
type Set struct {
	Strategy  string  `yaml:"strategy" json:"strategy"`
	Repos     []Entry `yaml:"repos" json:"repos"`
	Invariant string  `yaml:"invariant" json:"invariant"`
	Verify    *Verify `yaml:"verify" json:"verify"`
}

// Derive builds the set from a plan. coordinated is true when the developer chose a
// coordinated release on a card (doc 04 rule 5). Two repos that wait on each other cannot be
// ordered with one PR each, and are refused (M5-plan G2).
func Derive(p *plan.Plan, coordinated bool) (*Set, error) {
	var repos []string
	for _, t := range p.Tasks {
		if !t.Deferred && !slices.Contains(repos, t.Repo) {
			repos = append(repos, t.Repo)
		}
	}
	deps := map[string][]string{}
	for _, t := range p.Tasks {
		if !t.Deferred {
			deps[t.Repo] = union(deps[t.Repo], upstream(p, t))
		}
	}
	order, err := orders(repos, deps)
	if err != nil {
		return nil, err
	}
	s := &Set{Strategy: strategy(repos, p, coordinated), Invariant: Invariant}
	last := 0
	for _, r := range repos {
		s.Repos = append(s.Repos, Entry{Repo: r, Role: role(p, r), Order: order[r], DependsOn: nonNil(deps[r])})
		last = max(last, order[r])
	}
	s.Repos = append(s.Repos, cleanups(p, last+1)...)
	sort.SliceStable(s.Repos, func(i, j int) bool {
		a, b := s.Repos[i], s.Repos[j]
		return a.Order < b.Order || a.Order == b.Order && a.Repo < b.Repo
	})
	return s, nil
}

// upstream lists the other repos a task waits for (plan.Upstream).
func upstream(p *plan.Plan, t plan.Task) []string {
	var out []string
	for _, u := range p.Upstream(t.ID) {
		out = union(out, []string{u.Repo})
	}
	return out
}

// orders gives each repo its merge step: 1 for a repo that waits for none, otherwise one after
// the latest repo it waits for.
func orders(repos []string, deps map[string][]string) (map[string]int, error) {
	order := map[string]int{}
	visiting := map[string]bool{}
	var visit func(r string, path []string) (int, error)
	visit = func(r string, path []string) (int, error) {
		if o, ok := order[r]; ok {
			return o, nil
		}
		if visiting[r] {
			return 0, aerr.New(aerr.StateInvalid, cycle(append(path, r))).
				WithNextAction("split the change so one side expands first, or raise a coordinated-release card (doc 04 rule 5)")
		}
		visiting[r] = true
		o := 1
		for _, d := range deps[r] {
			do, err := visit(d, append(path, r))
			if err != nil {
				return 0, err
			}
			o = max(o, do+1)
		}
		visiting[r], order[r] = false, o
		return o, nil
	}
	for _, r := range repos {
		if _, err := visit(r, nil); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func cycle(path []string) string {
	start := slices.Index(path, path[len(path)-1])
	loop := path[start:]
	return strings.Join(loop[:len(loop)-1], " and ") + " wait on each other (" + strings.Join(loop, " → ") +
		"); one PR per repo cannot be ordered (M5-plan G2)"
}

// role is a repo's place in the contract chain, from its kinds of tasks built in this change.
func role(p *plan.Plan, repo string) string {
	has := func(kinds ...string) bool {
		return slices.ContainsFunc(p.Tasks, func(t plan.Task) bool {
			return t.Repo == repo && !t.Deferred && slices.Contains(kinds, t.Kind)
		})
	}
	switch {
	case has(plan.KindExpand, plan.KindMigrate):
		return Provider
	case has(plan.KindConsume):
		return Consumer
	}
	return Internal
}

func strategy(repos []string, p *plan.Plan, coordinated bool) string {
	switch {
	case coordinated:
		return Coordinated
	case len(repos) == 1 && !slices.ContainsFunc(p.Tasks, func(t plan.Task) bool { return t.Deferred }):
		return Single
	}
	return ExpandContract
}

// cleanups are the deferred tasks, one entry per repo after every merge of this change: they
// are a separate, later change (doc 04 rule 3).
func cleanups(p *plan.Plan, order int) []Entry {
	notes := map[string][]string{}
	var repos []string
	for _, t := range p.Tasks {
		if t.Deferred {
			if _, ok := notes[t.Repo]; !ok {
				repos = append(repos, t.Repo)
			}
			notes[t.Repo] = append(notes[t.Repo], t.ID+": "+t.Title)
		}
	}
	out := make([]Entry, 0, len(repos))
	for _, r := range repos {
		note := strings.Join(notes[r], "; ") + " (after consumers deploy)"
		out = append(out, Entry{Repo: r, Role: ProviderCleanup, Order: order, DependsOn: []string{}, Deferred: true, Note: &note})
	}
	return out
}

// Built lists the entries built in this change, in merge order.
func (s *Set) Built() []Entry {
	var out []Entry
	for _, e := range s.Repos {
		if !e.Deferred {
			out = append(out, e)
		}
	}
	return out
}

func union(a, b []string) []string {
	for _, x := range b {
		if !slices.Contains(a, x) {
			a = append(a, x)
		}
	}
	sort.Strings(a)
	return a
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Lines are the merge order, one line per step, as the plan summary and the PR bodies show it:
// "1. users (provider)", "2. gateway, orders (consumers; after users; either order)", and the
// deferred cleanup as a later change.
func (s *Set) Lines() []string {
	steps := map[int][]Entry{}
	var orders []int
	for _, e := range s.Repos {
		if _, ok := steps[e.Order]; !ok {
			orders = append(orders, e.Order)
		}
		steps[e.Order] = append(steps[e.Order], e)
	}
	var out []string
	for _, o := range orders {
		es := steps[o]
		names, after := make([]string, 0, len(es)), []string{}
		for _, e := range es {
			names = append(names, e.Repo)
			after = union(after, e.DependsOn)
		}
		if es[0].Deferred {
			for _, e := range es {
				out = append(out, "later, a separate change: "+e.Repo+" — "+*e.Note)
			}
			continue
		}
		line := strings.Join(names, ", ") + " (" + roles(es)
		if len(after) > 0 {
			line += "; after " + strings.Join(after, ", ")
		}
		if len(es) > 1 {
			line += "; either order"
		}
		out = append(out, strconv.Itoa(o)+". "+line+")")
	}
	return out
}

func roles(es []Entry) string {
	var rs []string
	for _, e := range es {
		if !slices.Contains(rs, e.Role) {
			rs = append(rs, e.Role)
		}
	}
	return strings.Join(rs, ", ")
}
