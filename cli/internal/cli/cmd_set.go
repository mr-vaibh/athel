package cli

import (
	"github.com/mr-vaibh/athel/cli/internal/changeset"
)

// setView is the change set of a change: the one fixed at plan approval, or, before approval, a
// preview derived from the current plan (ADR 035).
type setView struct {
	Change   string         `json:"change"`
	Approved bool           `json:"approved"`
	Set      *changeset.Set `json:"set"`
}

// cmdSetShow shows a change's set: repos, roles, merge order and the deferred cleanup.
func cmdSetShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("set show"), args, 1)
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
	if _, ok := s.ApprovedPlan(c); ok && c.ChangeSet != nil {
		return setView{Change: c.ID, Approved: true, Set: c.ChangeSet}, nil
	}
	p, err := s.LoadPlan(c.ID)
	if err != nil {
		return nil, err
	}
	set, err := changeset.Derive(p, false)
	if err != nil {
		return nil, err
	}
	return setView{Change: c.ID, Set: set}, nil
}
