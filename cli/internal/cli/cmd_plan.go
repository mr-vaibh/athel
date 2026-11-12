package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/blast"
	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	specpkg "github.com/mr-vaibh/athel/cli/internal/spec"
	"github.com/mr-vaibh/athel/cli/internal/state"
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
	data, err := r.readBlock(pos[1]) // a file, or "-" for stdin (the architect writes no files)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	if err := r.lintPlan(s, pos[0], data); err != nil {
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
	DryRun      bool     `json:"dry_run"`
	Migrated    []string `json:"migrated"`
	ConfigFiles []string `json:"config_files"`
}

// cmdMigrate upgrades config layers first: an old layer is read either way, but the store
// needs the effective config, and the changes are rewritten under it.
func cmdMigrate(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("migrate")
	dry := fs.Bool("dry-run", false, "list what would be upgraded without writing")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	if _, err := r.workspaceDir(); err != nil {
		return nil, err
	}
	paths, err := r.configPaths()
	if err != nil {
		return nil, err
	}
	res := migrateResult{DryRun: *dry, Migrated: []string{}}
	if res.ConfigFiles, err = config.MigrateLayers(paths, r.registry(), *dry); err != nil {
		return res, err
	}
	s, err := r.storeFor()
	if err != nil {
		return res, err
	}
	res.Migrated, err = s.Migrate(*dry)
	return res, err
}

// lintPlan applies doc 05's planning rules once the change has specs (M3 workflow): the plan
// must then be plan@2 and pass coverage, ordering and the task rules. A change without specs
// (the M1 flow) keeps accepting plan@1; the state machine requires specs before planned.
func (r *runCtx) lintPlan(s *state.Store, id string, data []byte) error {
	specs, err := r.loadSpecs(s, id)
	if err != nil || len(specs) == 0 {
		return err
	}
	p, err := plan.Decode(r.registry(), data)
	if err != nil {
		return draftError(err, "plan template")
	}
	if p.Schema != plan.SchemaID {
		return aerr.New(aerr.StateInvalid, id+" has specs, so its plan must be "+plan.SchemaID+" with a kind per task").
			WithNextAction("run athel plan template for the format")
	}
	risk, err := r.blastRisk(s, id)
	if err != nil {
		return err
	}
	if problems := plan.Lint(p, plan.LintContext{Criteria: criteria(specs), Risk: risk}); len(problems) > 0 {
		return aerr.New(aerr.StateInvalid, "plan rejected: "+strings.Join(problems, "; ")).
			WithNextAction("fix the plan and set it again")
	}
	return nil
}

func criteria(specs []*specpkg.Spec) []string {
	var out []string
	for _, sp := range specs {
		for _, c := range sp.Criteria {
			out = append(out, c.ID)
		}
	}
	return out
}

// blastRisk is the change's planned blast radius risk, or "" before a report exists.
func (r *runCtx) blastRisk(s *state.Store, id string) (string, error) {
	md, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(id), "blast-radius.md"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", aerr.Wrap(aerr.StateIO, "read blast-radius.md", err)
	}
	rep, err := blast.Extract(r.registry(), md)
	if err != nil || rep.Risk == nil {
		return "", err
	}
	return *rep.Risk, nil
}

// cmdPlanSummary is doc 05's one-screen summary before approval: tasks per repo in order,
// gates, env actions, risk, open assumptions and the estimate.
func cmdPlanSummary(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("plan summary"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	p, err := s.LoadPlan(pos[0])
	if err != nil {
		return nil, err
	}
	risk, err := r.blastRisk(s, p.Change)
	if err != nil {
		return nil, err
	}
	open, err := r.openAssumptions(s, p.Change)
	if err != nil {
		return nil, err
	}
	return plan.Summarise(p, risk, open), nil
}

// openAssumptions are what the human should see before approving: the specs' assumptions,
// unanswered cards, and defaults Athel applied for unanswered safe cards (doc 05: shown again
// at plan approval so they can be reversed).
func (r *runCtx) openAssumptions(s *state.Store, id string) ([]string, error) {
	specs, err := r.loadSpecs(s, id)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, sp := range specs {
		for _, a := range sp.Assumptions {
			out = append(out, sp.Service+" "+a.ID+": "+a.Text)
		}
	}
	dec, err := r.readDecisions(id)
	if err != nil {
		return nil, err
	}
	for _, e := range dec.Decisions {
		switch {
		case e.Card == nil:
		case e.Card.Answer == nil:
			out = append(out, e.Card.ID+" unanswered: "+e.Card.Question)
		case e.Card.Answer.By == "default":
			out = append(out, e.Card.ID+" default "+e.Card.Answer.Option+" applied, not answered: "+e.Card.Question)
		}
	}
	return out, nil
}

// cmdPlanDiff shows how the plan changed since the approved one (doc 05: a small change is
// re-presented as a plan diff, not a whole new plan). --against takes the approved plan's file.
func cmdPlanDiff(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("plan diff")
	against := fl.String("against", "", "the earlier plan file to compare with (required)")
	pos, err := parse(fl, args, 1)
	if err != nil {
		return nil, err
	}
	if *against == "" {
		return nil, usagef("plan diff: --against is required")
	}
	data, err := readFile(*against)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+*against, err)
	}
	old, err := plan.Decode(r.registry(), data)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	cur, err := s.LoadPlan(pos[0])
	if err != nil {
		return nil, err
	}
	return plan.Compare(old, cur), nil
}
