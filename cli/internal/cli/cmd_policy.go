package cli

import (
	"github.com/mr-vaibh/athel/cli/internal/config"
	"github.com/mr-vaibh/athel/cli/internal/policy"
)

// policySet loads the effective policy files (tools, environments, git) for this workspace.
func (r *runCtx) policySet() (*policy.Set, error) {
	if r.pol != nil {
		return r.pol, nil
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	home, err := config.DefaultHome(r.env.Getenv, r.env.UserHome)
	if err != nil {
		return nil, err
	}
	paths := config.Paths{Home: home}
	if ws, err := r.workspaceDir(); err == nil {
		paths.ProjectDir = ws.ConfigDir()
	}
	set, err := policy.Load(paths, r.registry(), cfg.Hash)
	if err != nil {
		return nil, err
	}
	r.pol = set
	return set, nil
}

type policyValidation struct {
	Valid      bool                  `json:"valid"`
	PolicyHash string                `json:"policy_hash"`
	Layers     map[string][]layerRef `json:"layers"`
}

func cmdPolicyValidate(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("policy validate"), args, 0); err != nil {
		return nil, err
	}
	set, err := r.policySet()
	if err != nil {
		return nil, err
	}
	out := policyValidation{Valid: true, PolicyHash: set.Hash, Layers: map[string][]layerRef{}}
	for file, layers := range set.Layers {
		for _, l := range layers {
			out.Layers[file] = append(out.Layers[file], layerRef{Name: l.Name, Path: l.Path})
		}
	}
	return out, nil
}

func cmdPolicyShow(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("policy show")
	fs.Bool("effective", true, "show the merged policy (default)")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	set, err := r.policySet()
	if err != nil {
		return nil, err
	}
	return set, nil
}
