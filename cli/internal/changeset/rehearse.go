package changeset

import (
	"slices"
	"strings"
)

// Steps are the states of the system the merge-order rehearsal checks (ADR 036): after each
// merge in the stated order, the repos merged so far. Where PRs share a step, each alone after
// the earlier steps, then all of them, since a human may merge any of them first. A coordinated
// release supports only the final state (doc 04 rule 5).
func (s *Set) Steps() [][]string {
	built := s.Built()
	if len(built) == 0 {
		return nil
	}
	all := make([]string, 0, len(built))
	for _, e := range built {
		all = append(all, e.Repo)
	}
	if s.Strategy == Coordinated {
		slices.Sort(all)
		return [][]string{all}
	}
	var out [][]string
	seen := map[string]bool{}
	add := func(state []string) {
		state = slices.Clone(state)
		slices.Sort(state)
		if key := strings.Join(state, ","); !seen[key] {
			seen[key] = true
			out = append(out, state)
		}
	}
	var before []string
	for o := 1; o <= built[len(built)-1].Order; o++ {
		var step []string
		for _, e := range built {
			if e.Order == o {
				step = append(step, e.Repo)
			}
		}
		if len(step) > 1 {
			for _, r := range step {
				add(append(slices.Clone(before), r))
			}
		}
		before = append(before, step...)
		add(before)
	}
	return out
}
