package hooks

import (
	"fmt"
	"slices"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/decisions"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// buildStop blocks the stop of a session whose build step's task is neither done nor blocked.
// ShouldBlockStop's loop protection applies: never while Claude Code is already continuing for a
// stop hook, and never more than StopBlockLimit times in a row. At the limit the step moves to
// blocked and a card asks the developer (doc 02 §Runtime model; M4-plan G13).
func buildStop(req *hookio.Request, d Deps, o *Outcome) {
	c := o.Change
	if c == nil || c.Step == nil || c.Step.Status != state.StepRunning || c.Step.Name != "build" || c.Step.Task == nil {
		return
	}
	task := *c.Step.Task
	if i := slices.IndexFunc(c.Build, func(t build.Task) bool { return t.ID == task }); i >= 0 && c.Build[i].Status == build.Done {
		return
	}
	if ShouldBlockStop(req, true, c.StopBlocks.Count) {
		n, err := d.Store.NoteStopBlock(c.ID, req.SessionID)
		if err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "stop: "+err.Error())
			return
		}
		o.Rule, o.Response.BlockStop = RuleStopBlock, true
		o.Response.BlockReason = fmt.Sprintf("Task %s is neither done nor blocked (stop %d of %d). Run its gates, fix what they report and run "+
			"`athel build done %s %s`, or end the step with `athel state exit %s build --result blocked --reason \"<why>\"`.",
			task, n, StopBlockLimit, c.ID, task, c.ID)
		return
	}
	if c.StopBlocks.Count >= StopBlockLimit {
		limitReached(req, d, c, task, o)
	}
}

// limitReached moves the build step to blocked and raises the card that asks the developer.
func limitReached(req *hookio.Request, d Deps, c *state.Change, task string, o *Outcome) {
	reason := fmt.Sprintf("the session stopped %d times with task %s neither done nor blocked", StopBlockLimit, task)
	if _, err := d.Store.ExitStep(c.ID, req.SessionID, "build", state.StepBlocked, reason); err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "stop: "+err.Error())
		return
	}
	draft := decisions.Draft{
		Rule: "A6", Step: "build", Question: "Task " + task + " stopped " + fmt.Sprint(StopBlockLimit) + " times without finishing. How should Athel go on?",
		Context: "Doc 02: after 3 blocked stops the step moves to blocked and asks. The change is blocked until you answer.",
		Options: []decisions.Option{
			{Key: "a", Label: "Stop and look at the task yourself", Consequence: "Athel writes nothing more for " + task + " until you answer again"},
			{Key: "b", Label: "Resume the task", Consequence: "the build step starts again for " + task + "; the stop count starts again"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "three stops in a row mean the agent cannot finish the task alone"},
		Default:        "a", DefaultSafe: true, Blocks: []string{task},
	}
	_, err := d.Store.UpdateChangeFile(c.ID, req.SessionID, "decisions.yaml", func(old []byte) ([]byte, error) {
		f, err := decisions.Decode(d.Store.Reg, c.ID, old)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(f.Open(""), func(x *decisions.Card) bool { return x.Question == draft.Question }) {
			if _, err := f.Add(draft); err != nil {
				return nil, err
			}
		}
		return f.Encode(d.Store.Reg)
	})
	if err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "stop card: "+err.Error())
		return
	}
	o.Rule, o.Response.SystemMessage = RuleStopBlock, "Athel: "+reason+"; the step is blocked and a card asks you how to go on."
}
