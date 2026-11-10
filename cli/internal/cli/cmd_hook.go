package cli

import (
	"errors"
	"io"
	"path/filepath"
	"time"

	athel "github.com/mr-vaibh/athel"
	"github.com/mr-vaibh/athel/cli/internal/adapters/claudecode"
	"github.com/mr-vaibh/athel/cli/internal/facts"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/hooks"
	"github.com/mr-vaibh/athel/cli/internal/observe"
	"github.com/mr-vaibh/athel/cli/internal/repos"
	"github.com/mr-vaibh/athel/cli/internal/scrub"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// maxHookInput bounds stdin; Claude Code hook payloads are far smaller.
const maxHookInput = 10 << 20

// rawOutput is printed verbatim: hook output must be exactly Claude Code's JSON.
type rawOutput string

// cmdHook handles `athel hook <event>`: payload on stdin, Claude Code's hook JSON on stdout,
// always exit 0 (any other code changes Claude Code's behaviour; M0-plan §3 D3). PreToolUse
// fails closed through the engine's rule 0; other failures are recorded, never turned into a
// block.
func cmdHook(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("hook"), args, 1)
	if err != nil {
		return nil, err
	}
	event, err := claudecode.EventFromCLIName(pos[0])
	if err != nil {
		return nil, usagef("hook: %v", err)
	}
	start := time.Now()
	payload, readErr := io.ReadAll(io.LimitReader(r.env.Stdin, maxHookInput+1))
	if readErr == nil && len(payload) > maxHookInput {
		readErr = errors.New("payload larger than 10 MiB")
	}
	var req *hookio.Request
	if readErr == nil {
		req, readErr = claudecode.ParseInput(event, payload)
	}
	deps := r.hookDeps(req)
	var outcome hooks.Outcome
	if readErr != nil {
		outcome = hooks.Malformed(event, readErr)
	} else {
		outcome = hooks.Handle(req, deps)
	}
	if deps.Store != nil {
		rec := hooks.NewAuditRecord(event, req, outcome, deps.ConfigHash, time.Since(start))
		if err := hooks.Record(deps.Store, rec); err != nil {
			_, _ = io.WriteString(r.env.Stderr, "athel hook: audit write failed: "+err.Error()+"\n") // stderr is the only channel left
		}
	}
	out, err := claudecode.FormatOutput(event, outcome.Response)
	if err != nil {
		return rawOutput("{}"), nil
	}
	return rawOutput(out), nil
}

// hookDeps finds the workspace from the payload's cwd (the hook process may run elsewhere).
func (r *runCtx) hookDeps(req *hookio.Request) hooks.Deps {
	d := hooks.Deps{Clock: r.env.Clock, EnvFile: r.env.Getenv("CLAUDE_ENV_FILE"),
		ClaudeVersion: r.env.ClaudeVersion, AppendEnv: func(path, line string) error { return fsutil.AppendLine(path, []byte(line)) },
		Decide: r.hookDecide, Scrub: scrub.New()}
	if minCC, err := MinClaudeCodeVersion(athel.PluginManifest); err == nil {
		d.MinClaudeCode = minCC
	}
	if req != nil && req.Cwd != "" && r.workspace == "" {
		r.workspace = req.Cwd
	}
	ws, err := state.Find(r.root())
	if err != nil {
		return d
	}
	r.ws = &ws
	if cfg, err := r.config(); err != nil {
		d.ConfigErr = err
	} else {
		d.ConfigHash = cfg.Hash
	}
	s := state.NewStore(ws, r.registry(), r.env.Clock)
	if r.cfg != nil {
		set := r.cfg.Settings()
		s.TTLMin, s.HeartbeatEvery, s.WarnRatio = set.LockTTLMin, set.HeartbeatEvery, set.WarnRatio
	}
	d.Store, r.store = s, s
	d.Observe = &observe.Store{Dir: filepath.Join(ws.KnowledgeDir(), "observations"), Reg: r.registry(), Clock: r.env.Clock, TTL: observe.DefaultTTL}
	if r.cfg != nil {
		set := r.cfg.Settings()
		d.Facts = &facts.Store{Dir: ws.KnowledgeDir(), Reg: r.registry(), Clock: r.env.Clock, Windows: set.FactWindows, ArchiveAfter: set.FactArchiveAfter}
		d.RefreshIndex = r.refreshStaleIndex
	}
	return d
}

// sessionIndexBudget keeps SessionStart well inside its hook timeout; a cut walk is retried at
// the next session.
const sessionIndexBudget = 3 * time.Second

// refreshStaleIndex rebuilds an existing index that has gone stale; with no index yet it does
// nothing, because indexing starts when the developer confirms roots.
func (r *runCtx) refreshStaleIndex() error {
	idx, ok, err := repos.Load(r.registry(), r.store.WS.RepoIndexFile())
	if err != nil || !ok || !idx.Stale(r.env.Clock.Now()) {
		return err
	}
	_, _, err = r.buildIndex(sessionIndexBudget)
	return err
}
