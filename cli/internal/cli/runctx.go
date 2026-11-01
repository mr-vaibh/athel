package cli

import (
	"os"

	"github.com/mr-vaibh/athel/cli/internal/config"
	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// runCtx carries the parsed global flags and lazily built services for one invocation.
type runCtx struct {
	env       Env
	json      bool
	workspace string
	session   string

	reg   *schema.Registry
	ws    *state.Workspace
	cfg   *config.Result
	store *state.Store
	pol   *policy.Set
}

func (r *runCtx) registry() *schema.Registry {
	if r.reg == nil {
		r.reg = schema.NewRegistry()
	}
	return r.reg
}

// root is the directory commands start from: --workspace, else the working directory.
func (r *runCtx) root() string {
	if r.workspace != "" {
		return r.workspace
	}
	return r.env.Cwd
}

func (r *runCtx) workspaceDir() (state.Workspace, error) {
	if r.ws == nil {
		ws, err := state.Find(r.root())
		if err != nil {
			return state.Workspace{}, err
		}
		r.ws = &ws
	}
	return *r.ws, nil
}

// config loads the effective configuration. Outside a workspace only the built-in, org and
// user layers apply.
func (r *runCtx) config() (*config.Result, error) {
	if r.cfg != nil {
		return r.cfg, nil
	}
	home, err := config.DefaultHome(r.env.Getenv, r.env.UserHome)
	if err != nil {
		return nil, err
	}
	paths := config.Paths{Home: home}
	if ws, err := r.workspaceDir(); err == nil {
		paths.ProjectDir = ws.ConfigDir()
	}
	cfg, err := config.Load(paths, r.registry())
	if err != nil {
		return nil, err
	}
	r.cfg = cfg
	return cfg, nil
}

// storeFor returns the change store of the workspace, tuned by the effective config.
func (r *runCtx) storeFor() (*state.Store, error) {
	if r.store != nil {
		return r.store, nil
	}
	ws, err := r.workspaceDir()
	if err != nil {
		return nil, err
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	s := state.NewStore(ws, r.registry(), r.env.Clock)
	set := cfg.Settings()
	s.TTLMin, s.HeartbeatEvery, s.WarnRatio = set.LockTTLMin, set.HeartbeatEvery, set.WarnRatio
	r.store = s
	return s, nil
}

// readFile is a seam for PRD input; os.ReadFile in production.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) } //nolint:gosec // the user names the PRD file to copy
