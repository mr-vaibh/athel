package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/state"
)

// resumeBrief is everything a brand-new session needs to continue a change from files alone
// (doc 02 §Runtime model rule 5; F-15, UC-10).
type resumeBrief struct {
	Change     stateView          `json:"change"`
	PRDPath    string             `json:"prd_path"`
	ReadOnly   bool               `json:"read_only"`
	Message    string             `json:"message"`
	NextAction string             `json:"next_action"`
	Recent     []state.Transition `json:"recent_history"`
	Others     []changeSummary    `json:"other_active_changes"`
}

const recentHistory = 5

func cmdResume(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("resume")
	takeover := fs.Bool("takeover", false, "take over a stale lock held by a dead session (logged)")
	fs.BoolVar(takeover, "take-over", false, "alias of --takeover")
	reason := fs.String("reason", "", "why the take-over is safe")
	pos, err := parse(fs, args, -1)
	if err != nil {
		return nil, err
	}
	if len(pos) > 1 {
		return nil, usagef("resume: expected at most 1 argument")
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := resolveChange(s, pos)
	if err != nil {
		return nil, err
	}
	brief := resumeBrief{Message: "this session now holds the lock on " + c.ID}
	if state.IsTerminal(c.State) {
		brief.ReadOnly, brief.Message = true, c.ID+" is "+string(c.State)+"; nothing to resume"
	} else {
		locked, err := s.AcquireLock(c.ID, session, state.AcquireOptions{Takeover: *takeover, Reason: *reason})
		switch {
		case isLockHeld(err):
			brief.ReadOnly, brief.Message = true, err.Error()
		case err != nil:
			return nil, err
		default:
			c = locked
		}
	}
	brief.Change = viewOf(c, r.env.Clock.Now())
	brief.PRDPath = filepath.Join(s.WS.ChangeDir(c.ID), "prd", c.PRD.Current+".md")
	brief.NextAction = nextAction(c, brief.ReadOnly)
	brief.Recent = c.History[max(len(c.History)-recentHistory, 0):]
	if brief.Recent == nil {
		brief.Recent = []state.Transition{}
	}
	brief.Others, err = otherActive(s, c.ID)
	return brief, err
}

// nextAction turns the recorded state into one plain instruction; it only restates what the
// files say, so a resumed session never depends on chat history.
func nextAction(c *state.Change, readOnly bool) string {
	switch {
	case readOnly && !state.IsTerminal(c.State):
		return "read-only: another session is working on " + c.ID + "; do not change it"
	case state.IsTerminal(c.State):
		return "none: " + c.ID + " is " + string(c.State)
	case c.State == state.Blocked:
		why := "unknown reason"
		if n := len(c.History); n > 0 {
			why = c.History[n-1].Reason
		}
		return fmt.Sprintf("resolve the blocker (%s), then return to %s with `athel state transition %s --to %s`",
			why, *c.BlockedFrom, c.ID, *c.BlockedFrom)
	case c.Step != nil && c.Step.Status == state.StepRunning:
		at := "the start of the step"
		if cp := state.LastCheckpoint(c); cp != nil && cp.Step != nil && *cp.Step == c.Step.Name {
			at = "checkpoint " + cp.Name
			if cp.Note != nil {
				at += " (" + *cp.Note + ")"
			}
		}
		return fmt.Sprintf("continue step %s (attempt %d) from %s", c.Step.Name, c.Step.Attempt, at)
	}
	next := make([]string, 0)
	for _, s := range state.NextStates(c) {
		next = append(next, string(s))
	}
	return fmt.Sprintf("%s is %s with no step running; next allowed states: %s", c.ID, c.State, strings.Join(next, ", "))
}

func otherActive(s *state.Store, id string) ([]changeSummary, error) {
	changes, err := s.List()
	if err != nil {
		return nil, err
	}
	out := []changeSummary{}
	for _, c := range changes {
		if c.ID != id && !state.IsTerminal(c.State) {
			out = append(out, summarize(c))
		}
	}
	return out, nil
}
