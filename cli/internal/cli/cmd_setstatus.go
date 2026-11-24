package cli

import (
	"encoding/json"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/changeset"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// setStatus is /athel:status for a change set (doc 04 §Cross-repo PR rules; M5-plan §5.5): each
// PR's state, review and CI, what to merge next, alarms, the rehearsal, the deferred cleanup,
// PRs dropped from the set, and the build time per repo.
type setStatus struct {
	Change string `json:"change"`
	changeset.Report
	Source     string             `json:"source"` // host, or recorded when the host could not be read
	Note       string             `json:"note,omitempty"`
	Rehearsal  string             `json:"rehearsal"`
	Dropped    []string           `json:"dropped"`
	MinutesPer map[string]float64 `json:"build_minutes_per_repo"`
}

// ghPRView is what gh pr view --json reports.
type ghPRView struct {
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	MergedAt       string `json:"mergedAt"`
	Checks         []struct {
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

func cmdSetStatus(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("set status"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	if c.ChangeSet == nil {
		return nil, aerr.New(aerr.StateInvalid, c.ID+" has no change set yet; it is derived when the plan is approved").
			WithNextAction("athel set show " + c.ID + " previews it")
	}
	now, source, note := r.hostPRs(c)
	p, _ := s.ApprovedPlan(c) // nil when the plan changed since: no times then
	out := setStatus{Change: c.ID, Report: c.ChangeSet.Status(now), Source: source, Note: note, Dropped: dropped(c), MinutesPer: buildMinutes(c, p)}
	why, err := r.rehearsalProblem(s, c)
	switch {
	case err != nil:
		out.Rehearsal = "unknown: " + err.Error()
	case why != "":
		out.Rehearsal = why
	case len(c.ChangeSet.Built()) < 2:
		out.Rehearsal = "not needed: one PR"
	default:
		out.Rehearsal = "passed at " + c.ChangeSet.Verify.At + " on the current heads"
	}
	return out, nil
}

// hostPRs reads each PR of the set from the host. When the host cannot be read, the states Athel
// last recorded stand in, and the note says so (M5-plan §5.8).
func (r *runCtx) hostPRs(c *state.Change) (map[string]changeset.PR, string, string) {
	now := map[string]changeset.PR{}
	for _, e := range c.ChangeSet.Built() {
		w, ok := repoWork(c, e.Repo)
		if !ok || w.PR == nil {
			continue
		}
		out, err := r.gh("pr", "view", path.Base(*w.PR), "--repo", ghRepo(w.Remote), "--json", "state,isDraft,reviewDecision,mergedAt,statusCheckRollup")
		var v ghPRView
		if err == nil {
			err = json.Unmarshal(out, &v)
		}
		if err != nil {
			return recorded(c), "recorded", "the host could not be read (" + err.Error() + "); these are the states Athel last saw"
		}
		now[e.Repo] = changeset.PR{Ref: w.PRRef(), State: prState(v), Review: v.ReviewDecision, CI: ciOf(v), MergedAt: v.MergedAt}
	}
	return now, "host", ""
}

func recorded(c *state.Change) map[string]changeset.PR {
	now := map[string]changeset.PR{}
	for _, e := range c.ChangeSet.Built() {
		p := changeset.PR{CI: "none"}
		if e.PR != nil {
			p.Ref = *e.PR
		}
		if e.State != nil {
			p.State = *e.State
		}
		now[e.Repo] = p
	}
	return now
}

func prState(v ghPRView) string {
	switch strings.ToLower(v.State) {
	case "merged":
		return "merged"
	case "closed":
		return "closed"
	}
	if v.IsDraft {
		return "draft"
	}
	return "ready"
}

// ciOf sums up the PR's checks: any failure fails, anything unfinished is pending.
func ciOf(v ghPRView) string {
	if len(v.Checks) == 0 {
		return "none"
	}
	ci := "success"
	for _, k := range v.Checks {
		switch {
		case slices.Contains([]string{"FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED"}, strings.ToUpper(k.Conclusion)):
			return "failure"
		case !strings.EqualFold(k.Status, "COMPLETED"):
			ci = "pending"
		}
	}
	return ci
}

// dropped are open PRs of repos no longer in the set (a card answer narrowed the change, G13):
// Athel does not close them; the developer decides.
func dropped(c *state.Change) []string {
	out := []string{}
	for _, w := range c.Repos {
		if w.PR != nil && !slices.ContainsFunc(c.ChangeSet.Built(), func(e changeset.Entry) bool { return e.Repo == w.Name }) {
			out = append(out, w.PRRef()+": dropped from "+c.ID+"; close it if it is no longer needed")
		}
	}
	return out
}

// buildMinutes is the build time per repo from the task records (start to done), for the
// efficiency measure (M5-plan A5, EC-2).
func buildMinutes(c *state.Change, p *plan.Plan) map[string]float64 {
	out := map[string]float64{}
	if p == nil {
		return out
	}
	repoOf := map[string]string{}
	for _, t := range p.Tasks {
		repoOf[t.ID] = t.Repo
	}
	for _, t := range c.Build {
		if t.Status != build.Done || t.DoneAt == nil {
			continue
		}
		start, err1 := clock.Parse(t.Started)
		end, err2 := clock.Parse(*t.DoneAt)
		if err1 != nil || err2 != nil || repoOf[t.ID] == "" {
			continue
		}
		out[repoOf[t.ID]] += end.Sub(start).Round(time.Second).Minutes()
	}
	return out
}
