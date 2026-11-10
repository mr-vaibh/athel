package cli

import (
	"flag"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

var _ flag.Value = (*stringList)(nil)

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// mutating returns the store and session for a command that writes state.
func (r *runCtx) mutating() (*state.Store, string, error) {
	if err := state.CheckSession(r.session); err != nil {
		return nil, "", err
	}
	s, err := r.storeFor()
	return s, r.session, err
}

func cmdStateTransition(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("state transition")
	to := fs.String("to", "", "target state (required)")
	by := fs.String("by", "", "human | athel (required)")
	reason := fs.String("reason", "", "why (required)")
	prompt := fs.String("prompt-id", "", "Claude Code prompt ID that asked for it (recorded for M1)")
	var evidence stringList
	fs.Var(&evidence, "evidence", "file inside the change folder that justifies it (repeatable)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	if *to == "" || *by == "" {
		return nil, usagef("state transition: --to and --by are required")
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	if err := r.stepGuard(s, pos[0], state.State(*to)); err != nil {
		return nil, err
	}
	c, err := s.Transition(pos[0], state.TransitionRequest{
		To: state.State(*to), By: *by, Reason: *reason, Evidence: evidence, Session: &session, PromptID: optional(*prompt),
	})
	if err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}

func cmdStateEnter(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("state enter")
	task := fs.String("task", "", "plan task ID, e.g. T-03")
	pos, err := parse(fs, args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.EnterStep(pos[0], session, pos[1], optional(*task))
	if err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}

func cmdStateExit(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("state exit")
	result := fs.String("result", "", "ok | blocked | failed (required)")
	reason := fs.String("reason", "", "required for blocked and failed")
	pos, err := parse(fs, args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.ExitStep(pos[0], session, pos[1], *result, *reason)
	if err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}

func cmdCheckpointSet(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("checkpoint set")
	note := fs.String("note", "", "what a fresh session needs to know to continue from here")
	pos, err := parse(fs, args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.AddCheckpoint(pos[0], session, pos[1], optional(*note), state.TriggerManual)
	if err != nil {
		return nil, err
	}
	return state.LastCheckpoint(c), nil
}

type checkpointsView struct {
	Last        *state.Checkpoint  `json:"last"`
	Checkpoints []state.Checkpoint `json:"checkpoints"`
}

func cmdCheckpointShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("checkpoint show"), args, 1)
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
	cps := c.Checkpoints
	if cps == nil {
		cps = []state.Checkpoint{}
	}
	return checkpointsView{Last: state.LastCheckpoint(c), Checkpoints: cps}, nil
}

func cmdLockAcquire(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("lock acquire")
	takeover := fs.Bool("takeover", false, "take over a stale lock (logged)")
	reason := fs.String("reason", "", "why the take-over is safe (required with --takeover)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.AcquireLock(pos[0], session, state.AcquireOptions{Takeover: *takeover, Reason: *reason})
	if err != nil {
		return nil, err
	}
	return state.Status(c, r.env.Clock.Now()), nil
}

type refreshResult struct {
	state.LockStatus
	Wrote bool `json:"wrote"`
}

func cmdLockRefresh(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("lock refresh")
	force := fs.Bool("force", false, "write even if the heartbeat is recent")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	wrote, err := s.RefreshLock(pos[0], session, *force)
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	return refreshResult{LockStatus: state.Status(c, r.env.Clock.Now()), Wrote: wrote}, nil
}

type releaseResult struct {
	Released []string `json:"released"`
}

func cmdLockRelease(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("lock release")
	allMine := fs.Bool("all-mine", false, "release every lock this session holds")
	pos, err := parse(fs, args, -1)
	if err != nil {
		return nil, err
	}
	if *allMine == (len(pos) == 1) || len(pos) > 1 {
		return nil, usagef("lock release: give either one change ID or --all-mine")
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	if *allMine {
		ids, err := s.ReleaseAllMine(session)
		return releaseResult{Released: ids}, err
	}
	before, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	if _, err := s.ReleaseLock(pos[0], session); err != nil {
		return nil, err
	}
	released := []string{}
	if before.Lock != nil {
		released = append(released, before.ID)
	}
	return releaseResult{Released: released}, nil
}

func cmdLockStatus(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("lock status"), args, -1)
	if err != nil {
		return nil, err
	}
	if len(pos) > 1 {
		return nil, usagef("lock status: expected at most 1 argument")
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	now := r.env.Clock.Now()
	if len(pos) == 1 {
		c, err := s.Get(pos[0])
		if err != nil {
			return nil, err
		}
		return []state.LockStatus{state.Status(c, now)}, nil
	}
	changes, err := s.List()
	if err != nil {
		return nil, err
	}
	out := []state.LockStatus{}
	for _, c := range changes {
		if c.Lock != nil {
			out = append(out, state.Status(c, now))
		}
	}
	return out, nil
}

func cmdBudgetAdd(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("budget add")
	cost := fs.Float64("cost-usd", 0, "cost to add, in US dollars")
	minutes := fs.Float64("minutes", 0, "wall time to add, in minutes")
	calls := fs.Int("tool-calls", 0, "tool calls to add")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.RecordUsage(pos[0], session, state.Usage{CostUSD: *cost, Minutes: *minutes, ToolCalls: *calls})
	if err != nil {
		return nil, err
	}
	return c.Budget, nil
}

func cmdBudgetShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("budget show"), args, 1)
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
	return c.Budget, nil
}

// isLockHeld reports whether err is another session's live lock.
func isLockHeld(err error) bool { return aerr.HasCode(err, aerr.StateLockHeld) }
