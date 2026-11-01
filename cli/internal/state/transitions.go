package state

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Actors that may request a transition.
const (
	ByHuman = "human"
	ByAthel = "athel"
)

// workingStates are every state that is neither terminal nor blocked (decision Q4: draft
// counts as a working state, so it can be blocked).
var workingStates = []State{
	Draft, Investigating, Specified, AwaitingDecisions, Planned, PlanApproved,
	Building, ReadyForPR, InReview, Verifying, UAT,
}

// AllStates lists every lifecycle state in lifecycle order.
var AllStates = append(slices.Clone(workingStates), Done, Blocked, Cancelled)

// forward is the fixed transition table between working states and into done
// (docs/plans/M0-plan.md §6.3). blocked and cancelled are handled by rule in Allowed.
var forward = map[State][]State{
	Draft:             {Investigating},
	Investigating:     {Specified},
	Specified:         {AwaitingDecisions},
	AwaitingDecisions: {Planned},
	Planned:           {PlanApproved},
	PlanApproved:      {Building, Planned},
	Building:          {ReadyForPR, Planned},
	ReadyForPR:        {InReview, Planned},
	InReview:          {Verifying},
	Verifying:         {UAT, Building},
	UAT:               {Done, Specified},
}

// humanOnly are the targets that need a human (doc 02, the two blue arrows). M1's hook layer
// will additionally require that the command came from a human prompt (Origin).
var humanOnly = map[State]bool{PlanApproved: true, Done: true}

// IsWorking reports whether s is a working state.
func IsWorking(s State) bool { return slices.Contains(workingStates, s) }

// IsTerminal reports whether s is done or cancelled.
func IsTerminal(s State) bool { return s == Done || s == Cancelled }

// RequiresHuman reports whether entering s needs --by human.
func RequiresHuman(s State) bool { return humanOnly[s] }

// Allowed reports whether from → to is legal. blockedFrom is the origin recorded while the
// change is blocked; blocked may only return there (or be cancelled).
func Allowed(from, to State, blockedFrom *State) bool {
	switch {
	case to == Cancelled:
		return !IsTerminal(from)
	case to == Blocked:
		return IsWorking(from)
	case from == Blocked:
		return blockedFrom != nil && to == *blockedFrom
	default:
		return slices.Contains(forward[from], to)
	}
}

// NextStates lists the legal targets from c's current state, in lifecycle order.
func NextStates(c *Change) []State {
	var out []State
	for _, to := range AllStates {
		if Allowed(c.State, to, c.BlockedFrom) {
			out = append(out, to)
		}
	}
	return out
}

// TransitionRequest is a request to move a change to another state.
type TransitionRequest struct {
	To       State
	By       string
	Reason   string
	Evidence []string // paths relative to the change directory
	Session  *string
	PromptID *string
}

// Transition moves change id to req.To if the fixed table allows it, appending a history
// entry with the hashes of the evidence files. Illegal and human-required rejections are
// recorded in the state event log and change nothing.
func (s *Store) Transition(id string, req TransitionRequest) (*Change, error) {
	var from State
	c, err := s.Update(id, func(c *Change) error {
		from = c.State
		if err := checkTransition(c, req); err != nil {
			return err
		}
		evidence, err := s.hashEvidence(c.ID, req.Evidence)
		if err != nil {
			return err
		}
		applyTransition(c, req, evidence, s.now())
		return nil
	})
	kind := EventTransition
	switch {
	case aerr.HasCode(err, aerr.StateIllegalTransition):
		kind = EventIllegalTransition
	case aerr.HasCode(err, aerr.PolicyHumanRequired):
		kind = EventHumanRequired
	case err != nil:
		return nil, err
	}
	ev := &Event{Kind: kind, Change: id, Session: req.Session, From: ptr(string(from)),
		To: ptr(string(req.To)), By: ptr(req.By), Reason: ptr(req.Reason)}
	if err != nil {
		ev.ErrorCode = ptr(string(mustCode(err)))
	}
	if logErr := s.logEvent(ev); logErr != nil {
		return nil, errorsJoin(err, logErr)
	}
	return c, err
}

func checkTransition(c *Change, req TransitionRequest) error {
	if req.By != ByHuman && req.By != ByAthel {
		return aerr.New(aerr.StateInvalid, fmt.Sprintf("--by must be %q or %q, got %q", ByHuman, ByAthel, req.By))
	}
	if strings.TrimSpace(req.Reason) == "" {
		return aerr.New(aerr.StateInvalid, "a transition needs a reason")
	}
	if !slices.Contains(AllStates, req.To) {
		return aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a lifecycle state", req.To))
	}
	if !Allowed(c.State, req.To, c.BlockedFrom) {
		return aerr.New(aerr.StateIllegalTransition,
			fmt.Sprintf("%s cannot move from %s to %s; allowed: %s", c.ID, c.State, req.To, joinStates(NextStates(c)))).
			WithNextAction("choose an allowed target state; never edit change.yaml by hand")
	}
	if RequiresHuman(req.To) && req.By != ByHuman {
		return aerr.New(aerr.PolicyHumanRequired, fmt.Sprintf("moving %s to %s needs --by human", c.ID, req.To)).
			WithNextAction("ask the human to approve; only a human may run this transition")
	}
	return nil
}

func applyTransition(c *Change, req TransitionRequest, evidence []Evidence, now string) {
	from := c.State
	c.BlockedFrom = nil
	if req.To == Blocked {
		c.BlockedFrom = ptr(from)
	}
	c.State = req.To
	c.History = append(c.History, Transition{
		Seq: len(c.History) + 1, From: from, To: req.To, By: req.By, At: now,
		Reason: req.Reason, Evidence: evidence,
		Origin: Origin{Session: req.Session, PromptID: req.PromptID},
	})
}

// hashEvidence hashes each evidence file. Paths are relative to the change directory and may
// not escape it, so a transition can only cite the change's own files.
func (s *Store) hashEvidence(id string, paths []string) ([]Evidence, error) {
	out := []Evidence{}
	base := s.WS.ChangeDir(id)
	for _, p := range paths {
		clean := filepath.Clean(p)
		if filepath.IsAbs(p) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("evidence %q must be a path inside the change folder", p))
		}
		sum, err := digest.File(filepath.Join(base, clean))
		if err != nil {
			return nil, err
		}
		out = append(out, Evidence{Path: filepath.ToSlash(clean), SHA256: sum})
	}
	return out, nil
}

func joinStates(states []State) string {
	if len(states) == 0 {
		return "none (terminal)"
	}
	parts := make([]string, len(states))
	for i, st := range states {
		parts[i] = string(st)
	}
	return strings.Join(parts, ", ")
}

func ptr[T any](v T) *T { return &v }
