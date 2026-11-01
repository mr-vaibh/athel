package cli

import (
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

type planView struct {
	Plan     *plan.Plan `json:"plan"`
	Approved bool       `json:"approved"`
	State    string     `json:"state"`
}

// cmdPlanSet stores a plan through the CLI (doc 04: .athel/changes/<id>/ is written only by the
// helper CLI). Changing an approved plan voids the approval.
func cmdPlanSet(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("plan set"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := readFile(pos[1])
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read plan file "+pos[1], err)
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.SetPlan(pos[0], session, data)
	if err != nil {
		return nil, err
	}
	p, err := s.LoadPlan(c.ID)
	if err != nil {
		return nil, err
	}
	_, approved := s.ApprovedPlan(c)
	return planView{Plan: p, Approved: approved, State: string(c.State)}, nil
}

func cmdPlanShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("plan show"), args, 1)
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
	p, err := s.LoadPlan(c.ID)
	if err != nil {
		return nil, err
	}
	_, approved := s.ApprovedPlan(c)
	return planView{Plan: p, Approved: approved, State: string(c.State)}, nil
}

type migrateResult struct {
	DryRun   bool     `json:"dry_run"`
	Migrated []string `json:"migrated"`
}

func cmdMigrate(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("migrate")
	dry := fs.Bool("dry-run", false, "list what would be upgraded without writing")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	ids, err := s.Migrate(*dry)
	return migrateResult{DryRun: *dry, Migrated: ids}, err
}
