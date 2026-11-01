package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// planFile is plan.yaml relative to the change folder.
const planFile = plan.FileName

// PlanFile is the path of a change's plan.yaml.
func (w Workspace) PlanFile(id string) string { return filepath.Join(w.ChangeDir(id), planFile) }

// LoadPlan reads and validates a change's plan; a missing plan is E-STATE-NOT-FOUND.
func (s *Store) LoadPlan(id string) (*plan.Plan, error) {
	data, err := os.ReadFile(s.WS.PlanFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, aerr.Wrap(aerr.StateNotFound, id+" has no plan.yaml", err)
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read plan of "+id, err)
	}
	return plan.Decode(s.Reg, data)
}

// ApprovedPlan returns the plan only if a human approved exactly this content.
func (s *Store) ApprovedPlan(c *Change) (*plan.Plan, bool) {
	if c.Approvals.Plan == nil {
		return nil, false
	}
	p, err := s.LoadPlan(c.ID)
	if err != nil || p.Hash == nil || *p.Hash != plan.ComputeHash(p) || *p.Hash != c.Approvals.Plan.Hash {
		return nil, false
	}
	return p, true
}

// approvePlan binds the plan_approved transition to the current plan's hash.
func (s *Store) approvePlan(c *Change, req TransitionRequest) error {
	p, err := s.LoadPlan(c.ID)
	if err != nil {
		return err
	}
	h := plan.ComputeHash(p)
	if p.Hash == nil || *p.Hash != h {
		return aerr.New(aerr.IntegrityState, "plan.yaml of "+c.ID+" was edited outside the athel CLI; set it again with `athel plan set`")
	}
	c.Approvals.Plan = &PlanApproval{Hash: h, By: ByHuman, At: s.now(), Session: req.Session, PromptID: req.PromptID}
	return nil
}

// SetPlan validates and stores a plan for change id. If it differs from an approved plan, the
// approval is void and a change that was approved, building or ready for a PR returns to
// planned (doc 02 §Transition rules, doc 05 FR-PLAN-004).
func (s *Store) SetPlan(id, session string, data []byte) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	p, err := plan.Decode(s.Reg, data)
	if err != nil {
		return nil, err
	}
	voided := false
	c, err := s.Update(id, func(c *Change) error {
		if p.Change != c.ID {
			return aerr.New(aerr.StateInvalid, "plan is for "+p.Change+", not "+c.ID)
		}
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; its plan cannot change")
		}
		now := s.Clock.Now()
		if err := s.claim(c, session, now); err != nil {
			return err
		}
		out, err := plan.Encode(p)
		if err != nil {
			return err
		}
		if err := fsutil.WriteFileAtomic(s.WS.PlanFile(c.ID), out); err != nil {
			return err
		}
		if c.Approvals.Plan != nil && c.Approvals.Plan.Hash != *p.Hash {
			c.Approvals.Plan, voided = nil, true
			if c.State == PlanApproved || c.State == Building || c.State == ReadyForPR {
				applyTransition(c, TransitionRequest{To: Planned, By: ByAthel, Reason: "plan changed after approval; approval void", Session: &session},
					[]Evidence{}, clock.Format(now))
			}
		}
		return nil
	})
	if err != nil || !voided {
		return c, err
	}
	reason := "plan changed after approval; approval void"
	return c, s.logEvent(&Event{Kind: EventTransition, Change: c.ID, Session: &session, To: ptr(string(c.State)), By: ptr(ByAthel), Reason: &reason})
}

// Migrate rewrites every change file in an older schema version to the current one.
func (s *Store) Migrate(dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(s.WS.ChangesDir())
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "list changes", err)
	}
	migrated := []string{}
	for _, e := range entries {
		data, err := os.ReadFile(s.WS.ChangeFile(e.Name()))
		if err != nil || !IsLegacy(data) {
			continue
		}
		migrated = append(migrated, e.Name())
		if dryRun {
			continue
		}
		if _, err := s.Update(e.Name(), func(*Change) error { return nil }); err != nil {
			return migrated, err
		}
	}
	return migrated, nil
}
