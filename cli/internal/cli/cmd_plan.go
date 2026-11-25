package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/blast"
	"github.com/mr-vaibh/athel/cli/internal/changeset"
	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
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
	rep, err := r.blastReport(s, id)
	if err != nil {
		return err
	}
	coordinated, err := r.coordinated(id)
	if err != nil {
		return err
	}
	ctx := plan.LintContext{Criteria: criteria(specs), Risk: riskOf(rep), Exists: r.baseHas(), Contracts: contracts(rep), SameRepo: r.sameRepo, Coordinated: coordinated,
		IsTest: func(repo, path string) bool { return r.testMatcher(repo)(path) }, MigrationDown: migrationDown}
	problems := plan.Lint(p, ctx)
	if _, err := changeset.Derive(p, coordinated); err != nil {
		var ae *aerr.Error
		if errors.As(err, &ae) {
			problems = append(problems, ae.Message+"; "+ae.NextAction)
		}
	}
	if len(problems) > 0 {
		return aerr.New(aerr.StateInvalid, "plan rejected: "+strings.Join(problems, "; ")).
			WithNextAction("fix the plan and set it again")
	}
	return nil
}

// migrationDown is the down migration the migration gate runs for an up migration (ADR 039).
func migrationDown(path string) (string, bool) {
	if !gates.IsMigration(path) || strings.HasSuffix(path, ".down.sql") {
		return "", false
	}
	return gates.DownFor(path), true
}

// baseHas reports whether a repo has a path on the base branch of Athel's cache clone, without
// fetching. A repo with no cache clone yet is not known, so the plan lint skips its paths.
func (r *runCtx) baseHas() func(repo, path string) (bool, bool) {
	pol, err := r.policySet()
	if err != nil {
		return nil // policy errors surface in the steps that need the policy
	}
	base := pol.Git.BaseBranch
	return func(repo, path string) (bool, bool) {
		cache, err := r.cachePath(r.resolveRepo(repo))
		if err != nil {
			return false, false
		}
		if code, _, err := gitro.Run(cache, "rev-parse", "--verify", "-q", "refs/heads/"+base); err != nil || code != 0 {
			return false, false
		}
		code, _, err := gitro.Run(cache, "cat-file", "-e", "refs/heads/"+base+":"+path)
		return err == nil && code == 0, err == nil
	}
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
	rep, err := r.blastReport(s, id)
	return riskOf(rep), err
}

// blastReport is the change's current blast radius report, nil before one exists.
func (r *runCtx) blastReport(s *state.Store, id string) (*blast.Report, error) {
	md, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(id), "blast-radius.md"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read blast-radius.md", err)
	}
	return blast.Extract(r.registry(), md)
}

func riskOf(rep *blast.Report) string {
	if rep == nil || rep.Risk == nil {
		return ""
	}
	return *rep.Risk
}

// contracts are the report's changed contracts, each with its consumers inside the change set,
// for the plan's multi-repo checks (M5-plan §5.2).
func contracts(rep *blast.Report) []plan.Contract {
	if rep == nil {
		return nil
	}
	var out []plan.Contract
	for _, d := range rep.Direct {
		c := plan.Contract{Item: d.Item, Provider: d.Service, Change: d.Change}
		for _, u := range rep.Consumers {
			if u.Of == d.Item && u.InChangeSet && !slices.Contains(c.Consumers, u.Consumer) {
				c.Consumers = append(c.Consumers, u.Consumer)
			}
		}
		out = append(out, c)
	}
	return out
}

// sameRepo says whether a blast radius service lives in a plan repo: the same name, or names
// that resolve to the same remote through the repo index.
func (r *runCtx) sameRepo(service, repo string) bool {
	return service == repo || r.resolveRepo(service) == r.resolveRepo(repo)
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
	sum := plan.Summarise(p, risk, open)
	coordinated, err := r.coordinated(p.Change)
	if err != nil {
		return nil, err
	}
	if set, err := changeset.Derive(p, coordinated); err == nil { // plan set refused a plan without one
		sum.MergeOrder = set.Lines()
	}
	return sum, nil
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
