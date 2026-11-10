package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	specpkg "github.com/mr-vaibh/athel/cli/internal/spec"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// loadSpecs reads every spec/<service>.md of a change, by service name.
func (r *runCtx) loadSpecs(s *state.Store, id string) ([]*specpkg.Spec, error) {
	dir := filepath.Join(s.WS.ChangeDir(id), "spec")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []*specpkg.Spec{}, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read the spec folder", err)
	}
	out := []*specpkg.Spec{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // a file in the change's own spec folder
		if err != nil {
			return nil, aerr.Wrap(aerr.StateIO, "read "+e.Name(), err)
		}
		sp, err := specpkg.Extract(r.registry(), b)
		if err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, "spec/"+e.Name(), err)
		}
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out, nil
}

type specChecked struct {
	Valid    bool     `json:"valid"`
	Service  string   `json:"service"`
	Problems []string `json:"problems"`
}

// checkSpec decodes a spec block and runs every check against the change: its current PRD
// index, decisions.yaml and its other specs.
func (r *runCtx) checkSpec(s *state.Store, id string, data []byte) (*specpkg.Spec, []string, error) {
	sp, err := specpkg.Decode(r.registry(), data)
	if err != nil {
		return nil, nil, err
	}
	x, c, err := r.loadPRDIndex(s, id, "")
	if err != nil {
		return nil, nil, err
	}
	dec, err := r.readDecisions(c.ID)
	if err != nil {
		return nil, nil, err
	}
	others, err := r.loadSpecs(s, c.ID)
	if err != nil {
		return nil, nil, err
	}
	problems := specpkg.Check(sp, specpkg.Context{Change: c.ID, PRD: x.Has, Others: others,
		Decision: func(d string) bool {
			for _, e := range dec.Decisions {
				if e.ID() == d {
					return true
				}
			}
			return false
		}})
	conv, err := r.conventionsOf(sp.Repo)
	switch {
	case aerr.HasCode(err, aerr.StateNotFound):
		problems = append(problems, "no conventions for "+sp.Repo+" yet: onboard the repo first (athel repos survey, then athel conventions save; FR-SPEC-006)")
	case err != nil:
		return nil, nil, err
	case !conv.Fresh:
		problems = append(problems, "the conventions of "+sp.Repo+" are stale ("+conv.Stale+"): survey and save them again")
	}
	if problems == nil {
		problems = []string{}
	}
	return sp, problems, nil
}

// cmdSpecValidate reports what save would decide, without writing.
func cmdSpecValidate(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("spec validate"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	sp, problems, err := r.checkSpec(s, pos[0], data)
	if err != nil {
		return nil, err
	}
	return specChecked{Valid: len(problems) == 0, Service: sp.Service, Problems: problems}, nil
}

type specSaved struct {
	Path    string `json:"path"`
	Service string `json:"service"`
}

// cmdSpecSave writes spec/<service>.md after every check passes (FR-SPEC-003/004/005).
func cmdSpecSave(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("spec save"), args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	sp, problems, err := r.checkSpec(s, pos[0], data)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "spec rejected: "+strings.Join(problems, "; ")).
			WithNextAction("fix the spec block and save again; `athel spec validate` checks without saving")
	}
	name := "spec/" + sp.Service + ".md"
	if _, err := s.SaveChangeFile(pos[0], session, name, specpkg.Render(sp, data)); err != nil {
		return nil, err
	}
	return specSaved{Path: filepath.Join(s.WS.ChangeDir(pos[0]), name), Service: sp.Service}, nil
}

type specList struct {
	Specs []*specpkg.Spec `json:"specs"`
}

func cmdSpecList(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("spec list"), args, 1)
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
	specs, err := r.loadSpecs(s, pos[0])
	return specList{Specs: specs}, err
}

func cmdSpecShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("spec show"), args, 2)
	if err != nil {
		return nil, err
	}
	out, err := cmdSpecList(r, pos[:1])
	if err != nil {
		return nil, err
	}
	for _, sp := range out.(specList).Specs {
		if sp.Service == pos[1] {
			return sp, nil
		}
	}
	return nil, aerr.New(aerr.StateNotFound, pos[0]+" has no spec for "+pos[1])
}
