package cli

import (
	"time"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/facts"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/repos"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// indexBudget bounds a walk started by a command; SessionStart uses a shorter one.
const indexBudget = time.Minute

type rootsProposal struct {
	Current    []string `json:"current"`
	Candidates []string `json:"candidates"`
	Confirm    string   `json:"confirm"`
}

// cmdReposRootsPropose lists default roots that exist; the developer confirms with /athel:roots.
func cmdReposRootsPropose(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("repos roots propose"), args, 0); err != nil {
		return nil, err
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	home, err := r.env.UserHome()
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "find the home folder", err)
	}
	current := cfg.Settings().SearchRoots
	return rootsProposal{Current: current, Candidates: repos.Propose(current, home),
		Confirm: "the developer types /athel:roots <folder>… in Claude Code; Athel never confirms roots itself"}, nil
}

type rootsSaved struct {
	Roots []string `json:"roots"`
	Grant string   `json:"grant"`
}

// cmdReposRootsSet saves search roots, only with the developer's own /athel:roots grant for
// exactly these roots (M2 plan Q7). The grant is spent first, so a failed save cannot be retried
// without the developer.
func cmdReposRootsSet(r *runCtx, args []string) (any, error) {
	roots, err := parse(r.newFlags("repos roots set"), args, -1)
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, usagef("repos roots set: name at least one root")
	}
	paths, err := r.configPaths()
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	roots = grants.CleanRoots(roots)
	key := grants.RootsKey(roots)
	g, err := s.Grants().Consume(key, grants.TargetRoots, session)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, aerr.New(aerr.PolicyHumanOrigin, "saving repo search roots needs the developer's own prompt /athel:roots with exactly these folders in this session, used once within "+grants.TTL.String()).
			WithNextAction("ask the developer to type /athel:roots followed by the folders, then run `" + grants.RootsCommand(roots) + "`")
	}
	if _, err := s.AuditLog().Append(audit.Record{Kind: audit.KindGrantUsed, Session: audit.Str(session), To: audit.Str(grants.TargetRoots),
		By: audit.Str(state.ByHuman), Reason: audit.Str("grant " + g.ID + " for " + key), Result: audit.ResultOK}); err != nil {
		return nil, err
	}
	if err := config.SetUserRoots(paths, r.registry(), roots); err != nil {
		return nil, err
	}
	return rootsSaved{Roots: roots, Grant: g.ID}, nil
}

type indexBuilt struct {
	IndexedAt string   `json:"indexed_at"`
	Roots     []string `json:"roots"`
	Repos     int      `json:"repos"`
	Complete  bool     `json:"complete"`
}

func cmdReposIndex(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("repos index"), args, 0); err != nil {
		return nil, err
	}
	idx, complete, err := r.buildIndex(indexBudget)
	if err != nil {
		return nil, err
	}
	return indexBuilt{IndexedAt: idx.IndexedAt, Roots: idx.Roots, Repos: len(idx.Repos), Complete: complete}, nil
}

// buildIndex walks the confirmed roots, records one structural fact per clone with a remote
// (its recipe re-checks the remote) and saves the index.
func (r *runCtx) buildIndex(budget time.Duration) (repos.Index, bool, error) {
	fstore, err := r.factStore()
	if err != nil {
		return repos.Index{}, false, err
	}
	configured := r.cfg.Settings().SearchRoots
	if len(configured) == 0 {
		return repos.Index{}, false, aerr.New(aerr.StateInvalid, "no confirmed repo search roots").
			WithNextAction("run athel repos roots propose, then ask the developer to type /athel:roots <folder>…")
	}
	home, err := r.env.UserHome()
	if err != nil {
		return repos.Index{}, false, aerr.Wrap(aerr.StateIO, "find the home folder", err)
	}
	roots := make([]string, 0, len(configured))
	for _, c := range configured {
		roots = append(roots, repos.Expand(c, home))
	}
	// The budget is wall-clock time; the recorded indexed_at comes from the injected clock.
	idx, complete := repos.Build(roots, r.env.Clock.Now(), time.Now().Add(budget), time.Now)
	for i := range idx.Repos {
		e := &idx.Repos[i]
		if e.Remote == nil {
			continue
		}
		f, _, err := fstore.Add(cloneFact(*e))
		if err != nil {
			return repos.Index{}, false, err
		}
		e.Fact = &f.ID
	}
	return idx, complete, repos.Save(r.registry(), r.store.WS.RepoIndexFile(), idx)
}

// cloneFact records where a remote is cloned; the recipe checks the clone still has that remote.
func cloneFact(e repos.Entry) facts.Draft {
	return facts.Draft{
		Subject: facts.Entity{"kind": "repo", "name": *e.Remote}, Predicate: "local_clone",
		Object: facts.Entity{"kind": "path", "name": e.Path}, Env: "any", Confidence: "likely",
		Sources:        []facts.Source{{Kind: "code", Ref: e.Path + "/.git/config"}},
		Verify:         []facts.Step{{"tool": "git", "action": "remote", "repo": e.Path, "url": "https://" + *e.Remote}},
		FreshnessClass: facts.Structural,
	}
}

func cmdReposShow(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("repos show"), args, 0); err != nil {
		return nil, err
	}
	ws, err := r.workspaceDir()
	if err != nil {
		return nil, err
	}
	idx, ok, err := repos.Load(r.registry(), ws.RepoIndexFile())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, aerr.New(aerr.StateNotFound, "no repo index yet").WithNextAction("run athel repos index")
	}
	return idx, nil
}

// repoResolver resolves recipe repos through the index; without one only checkout paths resolve.
func (r *runCtx) repoResolver() (func(string) (string, error), error) {
	idx, _, err := repos.Load(r.registry(), r.store.WS.RepoIndexFile())
	if err != nil {
		return nil, err
	}
	return idx.Resolve, nil
}
