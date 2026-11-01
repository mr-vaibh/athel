package state

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Step results for `athel state exit`.
const (
	StepRunning = "running"
	StepOK      = "ok"
	StepBlocked = "blocked"
	StepFailed  = "failed"
)

// stepStates says in which lifecycle states each workflow step (doc 02 skills) may run.
// Steps track work; only explicit transitions move the lifecycle (decision Q1, ADR 011), except
// that a step ending blocked moves the change to blocked.
var stepStates = map[string][]State{
	"start":         {Draft},
	"investigate":   {Investigating},
	"spec":          {Investigating, Specified},
	"blast-radius":  {Specified, AwaitingDecisions, Building, ReadyForPR},
	"decide":        {Specified, AwaitingDecisions},
	"plan":          {AwaitingDecisions, Planned},
	"git-preflight": {PlanApproved, Building},
	"build":         {Building},
	"review":        {Building},
	"ship":          {ReadyForPR, InReview},
	"verify":        {InReview, Verifying},
	"uat":           {UAT},
	"learn":         {UAT},
}

// Steps lists the known workflow steps, sorted.
func Steps() []string {
	out := make([]string, 0, len(stepStates))
	for s := range stepStates {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

var (
	taskRe       = regexp.MustCompile(`^T-[0-9]{2,}$`)
	checkpointRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

// EnterStep starts (or restarts) step on change id for session: it checks the step may run
// in the current state, takes the lock, bumps step_attempts and records the start.
func (s *Store) EnterStep(id, session, step string, task *string) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if _, ok := stepStates[step]; !ok {
		return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("unknown step %q (known: %s)", step, strings.Join(Steps(), ", ")))
	}
	if task != nil && !taskRe.MatchString(*task) {
		return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a task ID (want T-NN)", *task))
	}
	c, err := s.Update(id, func(c *Change) error {
		if !slices.Contains(stepStates[step], c.State) {
			return aerr.New(aerr.StateIllegalTransition, fmt.Sprintf("step %s cannot run while %s is %s", step, c.ID, c.State))
		}
		if c.Step != nil && c.Step.Status == StepRunning && c.Step.Name != step {
			return aerr.New(aerr.StateInvalid, fmt.Sprintf("step %s is still running on %s; exit it first", c.Step.Name, c.ID))
		}
		now := s.Clock.Now()
		if err := s.claim(c, session, now); err != nil {
			return err
		}
		c.StepAttempts[step]++
		c.Step = &Step{Name: step, Task: task, StartedAt: clock.Format(now), Attempt: c.StepAttempts[step], Status: StepRunning}
		c.StopBlocks.Count = 0
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, s.logEvent(&Event{Kind: EventStepEnter, Change: c.ID, Session: &session, Step: &step})
}

// ExitStep ends the running step with result ok, blocked or failed. blocked and failed need a
// reason; blocked also moves the change to the blocked state, remembering where it was.
func (s *Store) ExitStep(id, session, step, result, reason string) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if result != StepOK && result != StepBlocked && result != StepFailed {
		return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("--result must be ok, blocked or failed, got %q", result))
	}
	if result != StepOK && strings.TrimSpace(reason) == "" {
		return nil, aerr.New(aerr.StateInvalid, "a "+result+" step needs a reason")
	}
	c, err := s.Update(id, func(c *Change) error {
		if c.Step == nil || c.Step.Status != StepRunning || c.Step.Name != step {
			return aerr.New(aerr.StateInvalid, fmt.Sprintf("step %s is not running on %s", step, c.ID))
		}
		now := s.Clock.Now()
		if err := s.claim(c, session, now); err != nil {
			return err
		}
		stamp := clock.Format(now)
		c.Step.EndedAt, c.Step.Status = &stamp, result
		if reason != "" {
			c.Step.Reason = &reason
		}
		if result == StepBlocked {
			applyTransition(c, TransitionRequest{To: Blocked, By: ByAthel, Reason: "step " + step + " blocked: " + reason, Session: &session},
				[]Evidence{}, stamp)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ev := &Event{Kind: EventStepExit, Change: c.ID, Session: &session, Step: &step, Reason: &result}
	return c, s.logEvent(ev)
}

// Checkpoint triggers.
const (
	TriggerManual     = "manual"
	TriggerPreCompact = "precompact"
)

// AddCheckpoint appends a checkpoint inside the current step; resume restarts at the last one.
func (s *Store) AddCheckpoint(id, session, name string, note *string, trigger string) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if !checkpointRe.MatchString(name) {
		return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a checkpoint name (lowercase letters, digits, . _ -)", name))
	}
	return s.Update(id, func(c *Change) error {
		now := s.Clock.Now()
		if err := s.claim(c, session, now); err != nil {
			return err
		}
		var step *string
		if c.Step != nil && c.Step.Status == StepRunning {
			step = ptr(c.Step.Name)
		}
		c.Checkpoints = append(c.Checkpoints, Checkpoint{
			Name: name, Step: step, At: clock.Format(now), Note: note, Session: session, Trigger: trigger,
		})
		return nil
	})
}

// LastCheckpoint returns the checkpoint resume restarts at, or nil.
func LastCheckpoint(c *Change) *Checkpoint {
	if len(c.Checkpoints) == 0 {
		return nil
	}
	return &c.Checkpoints[len(c.Checkpoints)-1]
}
