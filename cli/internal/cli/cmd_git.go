package cli

import (
	"path/filepath"

	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/gitw"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

type worktreeMade struct {
	state.RepoWork
	Path    string `json:"path"`
	Created bool   `json:"created"`
	Reused  bool   `json:"reused_branch"`
}

// cmdGitWorktree makes Athel's worktree for one repo of an approved change (FR-GIT-001): cut from
// Athel's own cache clone, fetched first, on a branch named by git.yaml. The developer's clone is
// only read, to seed the cache. A repo that already has its worktree is returned as it is.
func cmdGitWorktree(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("git worktree"), args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	return r.ensureWorktree(s, session, pos[0], pos[1])
}

// ensureWorktree records and makes the worktree of repo for change id, or returns the recorded
// one. Making it fetches the cache clone first.
func (r *runCtx) ensureWorktree(s *state.Store, session, id, repo string) (worktreeMade, error) {
	c, err := s.Get(id)
	if err != nil {
		return worktreeMade{}, err
	}
	pol, err := r.policySet()
	if err != nil {
		return worktreeMade{}, err
	}
	reused := false
	tree := gitw.Tree{Git: r.networkGit}
	got, created, err := s.AddRepo(c.ID, session, repo, func(c *state.Change) (state.RepoWork, error) {
		cache, err := r.cacheClone(repo)
		if err != nil {
			return state.RepoWork{}, err
		}
		rel := filepath.Join("repos", repo+"@"+c.ID)
		added, err := tree.Add(gitw.Request{Cache: cache.Path, Dir: filepath.Join(s.WS.Root, rel), Base: pol.Git.BaseBranch,
			Branch: gitw.BranchName(pol.Git.BranchName, c.ID, c.Title), Change: c.ID, Prefix: gitw.Prefix(pol.Git.BranchName)})
		if err != nil {
			return state.RepoWork{}, err
		}
		reused = added.Reused
		return state.RepoWork{Name: repo, Remote: cache.Remote, Base: pol.Git.BaseBranch, BaseHead: added.BaseHead,
			Branch: added.Branch, Worktree: filepath.ToSlash(rel)}, nil
	})
	if err != nil {
		return worktreeMade{}, err
	}
	return worktreeMade{RepoWork: got, Path: filepath.Join(s.WS.Root, filepath.FromSlash(got.Worktree)), Created: created, Reused: reused}, nil
}

// networkGit runs git with the developer's credentials (fetches and remote reads).
func (r *runCtx) networkGit(dir string, args ...string) (int, string, error) {
	return gitro.RunEnv(gitro.NetworkEnv(r.env.Getenv), dir, args...)
}
