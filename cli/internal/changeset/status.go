package changeset

import (
	"fmt"
	"slices"
	"strings"
)

// PR is one PR of the set as the host reports it now (doc 04: /athel:status shows the whole
// set with each PR's review and CI state).
type PR struct {
	Repo     string `json:"repo"`
	Ref      string `json:"pr"`
	Order    int    `json:"order"`
	State    string `json:"state"`  // draft, ready, merged, closed, or "" before it is opened
	Review   string `json:"review"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED or ""
	CI       string `json:"ci"`     // success, failure, pending or none
	MergedAt string `json:"merged_at,omitempty"`
}

// Report is the set's status: every PR, what to merge next, alarms when the order was broken,
// and the deferred cleanup.
type Report struct {
	Strategy string   `json:"strategy"`
	PRs      []PR     `json:"prs"`
	Next     []string `json:"next"`
	Alarms   []string `json:"alarms"`
	Cleanup  []string `json:"cleanup"`
}

// Status reads the set against what the host reports, keyed by repo (M5-plan §5.5, G8). Athel
// never merges, reverts or closes anything; it says what to do.
func (s *Set) Status(now map[string]PR) Report {
	rep := Report{Strategy: s.Strategy, PRs: []PR{}, Next: []string{}, Alarms: []string{}, Cleanup: []string{}}
	built := s.Built()
	merged := func(repo string) bool { return now[repo].State == "merged" }
	name := func(repo string) string {
		if p := now[repo].Ref; p != "" {
			return p
		}
		return repo
	}
	for _, e := range built {
		p := now[e.Repo]
		p.Repo, p.Order = e.Repo, e.Order
		rep.PRs = append(rep.PRs, p)
	}
	all := !slices.ContainsFunc(built, func(e Entry) bool { return !merged(e.Repo) })
	if s.Strategy == Coordinated {
		rep.Next, rep.Alarms = coordinatedNext(built, merged, name)
	} else {
		for _, e := range built {
			waiting := slices.ContainsFunc(e.DependsOn, func(d string) bool { return !merged(d) })
			if !merged(e.Repo) && !waiting && now[e.Repo].State == "ready" {
				rep.Next = append(rep.Next, name(e.Repo))
			}
		}
		rep.Alarms = outOfOrder(built, now, merged, name)
	}
	for _, e := range s.Repos {
		if !e.Deferred {
			continue
		}
		if all {
			rep.Cleanup = append(rep.Cleanup, e.Repo+": "+*e.Note+": when every consumer is deployed, run /athel:followup to draft it")
		} else {
			rep.Cleanup = append(rep.Cleanup, e.Repo+": "+*e.Note+": waits until every PR of the set is merged and deployed")
		}
	}
	return rep
}

func coordinatedNext(built []Entry, merged func(string) bool, name func(string) string) ([]string, []string) {
	var next, done []string
	for _, e := range built {
		if merged(e.Repo) {
			done = append(done, name(e.Repo))
		} else {
			next = append(next, name(e.Repo))
		}
	}
	if len(done) > 0 && len(next) > 0 {
		return next, []string{"coordinated release partly merged (" + strings.Join(done, ", ") + "): merge " +
			strings.Join(next, ", ") + " in the same deploy window; the states in between are not supported"}
	}
	if next == nil {
		next = []string{}
	}
	return next, []string{}
}

// outOfOrder are alarms for PRs merged before a PR they must follow (G8).
func outOfOrder(built []Entry, now map[string]PR, merged func(string) bool, name func(string) string) []string {
	out := []string{}
	for _, e := range built {
		if !merged(e.Repo) {
			continue
		}
		for _, d := range e.DependsOn {
			switch {
			case !merged(d):
				out = append(out, fmt.Sprintf("%s was merged before %s, which it must follow: merge %s now; until it is, %s runs against %s's old contract",
					name(e.Repo), name(d), name(d), e.Repo, d))
			case now[d].MergedAt > now[e.Repo].MergedAt:
				out = append(out, fmt.Sprintf("%s was merged before %s (%s, then %s); both are merged now, so check what was deployed in between",
					name(e.Repo), name(d), now[e.Repo].MergedAt, now[d].MergedAt))
			}
		}
	}
	return out
}
