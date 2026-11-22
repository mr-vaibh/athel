package cli

import (
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	specpkg "github.com/mr-vaibh/athel/cli/internal/spec"
	"github.com/mr-vaibh/athel/cli/internal/state"
	"github.com/mr-vaibh/athel/cli/internal/trace"
)

// buildTrace links the change's current PRD, specs and plan (none yet is fine).
func (r *runCtx) buildTrace(s *state.Store, id string) (trace.Trace, *plan.Plan, error) {
	x, c, err := r.loadPRDIndex(s, id, "")
	if err != nil {
		return trace.Trace{}, nil, err
	}
	specs, err := r.loadSpecs(s, c.ID)
	if err != nil {
		return trace.Trace{}, nil, err
	}
	p, err := s.LoadPlan(c.ID)
	if aerr.HasCode(err, aerr.StateNotFound) {
		p, err = nil, nil
	}
	if err != nil {
		return trace.Trace{}, nil, err
	}
	return trace.Build(c.ID, *x, specs, p), p, nil
}

// cmdTrace writes traceability.md (doc 05: PRD → REQ → AC → task; FR-SPEC-003, FR-PLAN-002).
func cmdTrace(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("trace"), args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	t, _, err := r.buildTrace(s, pos[0])
	if err != nil {
		return nil, err
	}
	if _, err := s.SaveChangeFile(pos[0], session, "traceability.md", trace.Render(t)); err != nil {
		return nil, err
	}
	return t, nil
}

// stepGuard checks that the work a state stands for exists and passes its checks (doc 05;
// ADR 021), so no change reaches specified or planned on the model's word alone.
func (r *runCtx) stepGuard(s *state.Store, id string, to state.State) error {
	switch to {
	case state.Specified:
		return r.specifiedGuard(s, id)
	case state.Planned:
		return r.plannedGuard(s, id)
	case state.PlanApproved:
		return r.signoffGuard(s, id)
	case state.Building:
		return r.buildingGuard(s, id)
	}
	return nil
}

// signoffGuard: a CRITICAL blast radius needs the developer's sign-off of the current report
// before the plan can be approved (doc 05 §Risk rules).
func (r *runCtx) signoffGuard(s *state.Store, id string) error {
	risk, err := r.blastRisk(s, id)
	if err != nil || risk != "CRITICAL" {
		return err
	}
	ok, err := r.signedOff(s, id)
	if err != nil {
		return err
	}
	if !ok {
		return guardError(id, state.PlanApproved, []string{"the blast radius is CRITICAL and the current report has no sign-off (/athel:signoff " + id + ")"})
	}
	return nil
}

func (r *runCtx) specifiedGuard(s *state.Store, id string) error {
	x, c, err := r.loadPRDIndex(s, id, "")
	if err != nil {
		return err
	}
	var problems []string
	if _, err := r.readChangeFile(s, c.ID, "prd/review-"+x.Version+".md"); err != nil {
		problems = append(problems, "the PRD "+x.Version+" has no review yet (athel prd review save)")
	}
	specs, err := r.loadSpecs(s, c.ID)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		problems = append(problems, "no spec yet (athel spec save)")
	}
	for _, sp := range specs {
		block, err := encodeSpec(sp)
		if err != nil {
			return err
		}
		_, p, err := r.checkSpec(s, c.ID, block)
		if err != nil {
			return err
		}
		for _, msg := range p {
			problems = append(problems, sp.Service+": "+msg)
		}
	}
	return guardError(c.ID, state.Specified, problems)
}

func (r *runCtx) plannedGuard(s *state.Store, id string) error {
	t, p, err := r.buildTrace(s, id)
	if err != nil {
		return err
	}
	problems := append([]string{}, t.Missing...)
	if p != nil {
		data, err := plan.Encode(p)
		if err != nil {
			return err
		}
		if err := r.lintPlan(s, id, data); err != nil {
			problems = append(problems, err.Error())
		}
	}
	dec, err := r.readDecisions(id)
	if err != nil {
		return err
	}
	for _, card := range dec.Open("") {
		for _, b := range card.Blocks {
			if b == "spec" || b == "plan" {
				problems = append(problems, card.ID+" is unanswered and blocks the "+b+" step")
			}
		}
	}
	return guardError(id, state.Planned, problems)
}

func guardError(id string, to state.State, problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return aerr.New(aerr.StateInvalid, id+" cannot move to "+string(to)+" yet: "+strings.Join(problems, "; ")).
		WithNextAction("finish the step's work, then transition again")
}

// readChangeFile reads a file of the change folder.
func (r *runCtx) readChangeFile(s *state.Store, id, name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(id), name)) //nolint:gosec // a named file of the change folder
	if err != nil {
		return nil, aerr.Wrap(aerr.StateNotFound, id+" has no "+name, err)
	}
	return b, nil
}

// encodeSpec is a stored spec's block again, for re-checking it against the current PRD,
// decisions and conventions.
func encodeSpec(sp *specpkg.Spec) ([]byte, error) {
	b, err := yaml.Marshal(sp)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, "encode spec "+sp.Service, err)
	}
	return b, nil
}

// Templates the architect starts from (M3 plan §4.1b): valid examples with every allowed value.
func cmdSpecTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "spec template", specpkg.Template)
}

func cmdDecideTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "decide template", decisions.Template)
}

func cmdPlanTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "plan template", plan.Template)
}

func printTemplate(r *runCtx, args []string, name, tmpl string) (any, error) {
	if _, err := parse(r.newFlags(name), args, 0); err != nil {
		return nil, err
	}
	return rawOutput(strings.TrimRight(tmpl, "\n")), nil
}
