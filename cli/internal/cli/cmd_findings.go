package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/findings"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	prdpkg "github.com/mr-vaibh/athel/cli/internal/prd"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Doc 07's side findings: any agent records one (FR-FIND-001), none is fixed inside the change
// (FR-FIND-002, rule 7), and at PR time the developer chooses for each (FR-FIND-003).

func (r *runCtx) readFindings(s *state.Store, id string) (*findings.File, error) {
	f := &findings.File{Schema: findings.SchemaID, Findings: []findings.Finding{}}
	b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(id), "findings.yaml"))
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read findings.yaml", err)
	}
	if err := r.registry().ValidateYAML(findings.SchemaID, b); err != nil {
		return nil, err
	}
	return f, yaml.Unmarshal(b, f)
}

// editFindings changes findings.yaml under the change lock.
func (r *runCtx) editFindings(id string, fn func(f *findings.File) error) error {
	s, session, err := r.mutating()
	if err != nil {
		return err
	}
	_, err = s.UpdateChangeFile(id, session, "findings.yaml", func([]byte) ([]byte, error) {
		f, err := r.readFindings(s, id)
		if err != nil {
			return nil, err
		}
		if err := fn(f); err != nil {
			return nil, err
		}
		b, err := yaml.Marshal(f)
		if err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, "encode findings.yaml", err)
		}
		return b, r.registry().ValidateYAML(findings.SchemaID, b)
	})
	return err
}

// cmdFindingAdd records a side finding read from a file or stdin.
func cmdFindingAdd(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("finding add"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	var d findings.Draft
	if err := decodeStrict(data, &d, "finding"); err != nil {
		return nil, draftError(err, "")
	}
	var added findings.Finding
	err = r.editFindings(pos[0], func(f *findings.File) error {
		var err error
		added, err = f.Add(d)
		return err
	})
	return added, err
}

type findingList struct {
	Summary  string             `json:"summary"`
	Findings []findings.Finding `json:"findings"`
}

// cmdFindingList shows the findings ranked, with doc 07's one-line summary.
func cmdFindingList(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("finding list"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	if _, err := s.Get(pos[0]); err != nil {
		return nil, err
	}
	f, err := r.readFindings(s, pos[0])
	if err != nil {
		return nil, err
	}
	return findingList{Summary: f.Summary(), Findings: f.Ranked()}, nil
}

// cmdFindingTriage records the developer's dispositions, exactly as their /athel:triage prompt
// named them; `all=<d>` sets every finding not decided yet.
func cmdFindingTriage(r *runCtx, args []string) (any, error) {
	if len(args) < 2 {
		return nil, usagef("usage: athel finding triage CHG-NNN SF-1=fix_pr|ticket|ignore … | all=…")
	}
	choices, ok := grants.ParseChoices(args[1:])
	if !ok {
		return nil, usagef("finding triage: each choice is SF-n=fix_pr|ticket|ignore, once, or all=… alone")
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	g, err := s.Grants().Consume(grants.TriageKey(args[0], choices), grants.TargetTriage, session)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, aerr.New(aerr.PolicyHumanOrigin, "only the developer chooses what happens to side findings, with /athel:triage "+args[0]+" in this session").
			WithNextAction("show the developer athel finding list " + args[0] + " and ask them to type /athel:triage")
	}
	var out *findings.File
	err = r.editFindings(args[0], func(f *findings.File) error {
		picked := map[string]string{}
		for _, c := range choices {
			id, d, _ := strings.Cut(c, "=")
			if id != "all" {
				picked[id] = d
				continue
			}
			for _, u := range f.Undecided() {
				picked[u] = d
			}
		}
		out = f
		return f.Triage(picked, state.ByHuman, clock.Format(r.env.Clock.Now()))
	})
	if err != nil {
		return nil, err
	}
	return findingList{Summary: out.Summary(), Findings: out.Ranked()}, nil
}

// prFindings are the side-finding lines of a PR body: each finding and what the developer chose.
func (r *runCtx) prFindings(s *state.Store, id string) ([]string, error) {
	f, err := r.readFindings(s, id)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, x := range f.Ranked() {
		choice := "not decided yet"
		if x.Disposition != nil {
			choice = *x.Disposition
		}
		out = append(out, x.ID+" ("+x.Category+", "+x.Severity+") "+x.Location+": "+x.Evidence+" → "+choice)
	}
	return out, nil
}

// cmdFindingFix turns a finding the developer chose to fix into its own change, <parent>-F<n>,
// linked to its parent (doc 07 FR-FIND-004): it then goes through the same spec, plan, gates,
// review and PR as any change, on its side_finding_branch.
func cmdFindingFix(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("finding fix"), args, 2)
	if err != nil {
		return nil, err
	}
	s, _, err := r.mutating()
	if err != nil {
		return nil, err
	}
	f, err := r.readFindings(s, pos[0])
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(f.Findings, func(x findings.Finding) bool { return x.ID == pos[1] })
	switch {
	case i < 0:
		return nil, aerr.New(aerr.StateNotFound, pos[0]+" has no side finding "+pos[1])
	case f.Findings[i].Disposition == nil || *f.Findings[i].Disposition != findings.FixPR:
		return nil, aerr.New(aerr.StateInvalid, pos[1]+" is not chosen for a fix PR").WithNextAction("ask the developer: /athel:triage " + pos[0] + " " + pos[1] + "=fix_pr")
	case f.Findings[i].Child != "":
		return nil, aerr.New(aerr.ConflictVersion, pos[1]+" is already being fixed in "+f.Findings[i].Child)
	}
	x := f.Findings[i]
	n, _ := strconv.Atoi(strings.TrimPrefix(x.ID, "SF-")) // IDs are SF-<n>
	prd := []byte("# Fix " + x.ID + " of " + pos[0] + "\n\n" + x.Evidence + "\n\nWhere: " + x.Location + " (" + x.Category + ", " + x.Severity +
		").\n\nSuggested fix: " + x.SuggestedFix + "\n")
	set := r.cfg.Settings()
	child, err := s.Create(state.NewChange{Title: "Fix " + x.ID + ": " + x.Category + " in " + x.Location, CreatedBy: cmpOr(set.Identity, "unknown"),
		PRD: prd, PRDSource: "text", Parent: pos[0], Finding: n,
		IndexFor: func(id, sha string) []byte { return prdpkg.Encode(prdpkg.Build(id, "v1", sha, prd, nil)) },
		Budget:   state.Budget{MaxCostUSD: set.BudgetMaxCostUSD, MaxHours: set.BudgetMaxHours, MaxToolCalls: set.BudgetMaxToolCalls}})
	if err != nil {
		return nil, err
	}
	err = r.editFindings(pos[0], func(f *findings.File) error {
		f.Findings[i].Child = child.ID
		return nil
	})
	return summarize(child), err
}
