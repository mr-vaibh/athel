package cli

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/review"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Doc 06 FR-BUILD-014: gate waivers require a human, a reason and an expiry, and appear in the PR
// body. Only the developer's own /athel:waive prompt creates the grant `athel gate waive` needs.

// maxWaiver is how far ahead a waiver may run; a longer one would outlive any review of it.
const maxWaiver = 30 * 24 * time.Hour

var findingID = regexp.MustCompile(`^R[1-9][0-9]*-[0-9]{2,}$`)

func cmdGateWaive(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("gate waive")
	until := fs.String("until", "", "the last day the waiver holds, YYYY-MM-DD (required)")
	reason := fs.String("reason", "", "why the gate or finding may pass (required)")
	pos, err := parse(fs, args, 3)
	if err != nil {
		return nil, err
	}
	g, err := r.gateTask(pos[:2], "gate waive")
	if err != nil {
		return nil, err
	}
	if err := r.waivable(g, pos[2], *until, *reason); err != nil {
		return nil, err
	}
	key := grants.WaiveKey(grants.Waiver{Change: g.c.ID, Task: g.task.ID, Subject: pos[2], Until: *until, Reason: *reason})
	gr, err := g.s.Grants().Consume(key, grants.TargetWaive, g.session)
	if err != nil {
		return nil, err
	}
	if gr == nil {
		return nil, aerr.New(aerr.PolicyHumanOrigin, "waiving "+pos[2]+" needs the developer's own prompt, with the same task, expiry and reason").
			WithNextAction("ask the developer to type /athel:waive " + g.c.ID + " " + g.task.ID + " " + pos[2] + " until=" + *until + " '<reason>'")
	}
	if findingID.MatchString(pos[2]) {
		if _, err := r.changeRound(pos[:2], func(_ gateTask, rd *review.Round) error { return rd.Waive(pos[2], *reason) }); err != nil {
			return nil, err
		}
	}
	w := state.Waiver{Task: g.task.ID, Subject: pos[2], By: state.ByHuman, Reason: *reason, Until: *until, Grant: gr.ID, At: clock.Format(r.env.Clock.Now())}
	if _, err := g.s.AddWaiver(g.c.ID, g.session, w); err != nil {
		return nil, err
	}
	return w, nil
}

// waivable refuses what doc 06 never lets a human waive, and a waiver without a reason or with
// an expiry outside today … 30 days ahead.
func (r *runCtx) waivable(g gateTask, subject, until, reason string) error {
	refuse := func(msg string) error {
		return aerr.New(aerr.StateInvalid, "waiver refused: "+msg).WithNextAction("fix the gate or the finding instead")
	}
	day, err := time.Parse(time.DateOnly, until)
	today := r.env.Clock.Now().UTC().Truncate(24 * time.Hour)
	switch {
	case strings.TrimSpace(reason) == "":
		return refuse("a waiver needs a reason")
	case err != nil:
		return refuse("--until is a date, YYYY-MM-DD")
	case day.Before(today):
		return refuse("the expiry " + until + " is in the past")
	case day.After(today.Add(maxWaiver)):
		return refuse("the expiry " + until + " is more than 30 days ahead")
	case findingID.MatchString(subject):
		return nil // review.Waive checks the severity
	}
	list, err := r.gateList(g)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(x gates.Gate) bool { return x.ID == subject })
	switch {
	case i < 0:
		return refuse("task " + g.task.ID + " has no gate " + subject)
	case list[i].Waivable == gates.Never:
		return refuse(subject + " is never waivable (doc 06)")
	case list[i].Waivable == gates.SecurityOwner:
		return refuse(subject + " is waived only by the security owner, whom Athel cannot confirm yet; the security owner approves it outside Athel (M4-plan G18)")
	}
	return nil
}

// applyWaiver turns a red result into "waived" while a waiver holds, for a gate a human may waive.
func applyWaiver(c *state.Change, task string, gate gates.Gate, day string, o *gateOutcome) {
	if gate.Waivable != gates.Human || (o.Status != gates.Fail && o.Status != "unavailable") || !c.Waived(task, gate.ID, day) {
		return
	}
	o.Summary = "waived by a human; the gate said: " + o.Status + ": " + o.Summary
	o.Status = gates.Waived
}
