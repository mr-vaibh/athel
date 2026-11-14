// Package gitw is Athel's git that writes (FR-GIT-001): worktrees and branches inside Athel's own
// workspace, cut from its cache clones (M2). It never touches the developer's clones; reading
// them stays in gitro.
package gitw

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Tree makes worktrees. Git runs git in a folder with the developer's credentials (as
// repos.Cache.Git): fetching a branch or reading the remote reaches it.
type Tree struct {
	Git func(dir string, args ...string) (int, string, error)
}

// Request is one worktree to add.
type Request struct {
	Cache  string // the bare cache clone
	Dir    string // the worktree folder, workspace/repos/<repo>@<change>
	Base   string // the base branch
	Branch string // the wanted branch name (BranchName)
	Change string // the change ID the branch's commits must carry to be reused
	Prefix string // the fixed prefix of Athel's branch names, kept out of the cache's fetch
}

// Added is the worktree made: its branch (the wanted name, or one with a suffix), the base
// commit it starts from, and whether an earlier run's branch of this change was reused.
type Added struct {
	Branch   string `json:"branch"`
	BaseHead string `json:"base_head"`
	Reused   bool   `json:"reused"`
}

// maxSuffix bounds the branch names tried: name, name-2 … name-9.
const maxSuffix = 9

// changeTrailer is the commit trailer git.yaml puts on every Athel commit (doc 04).
const changeTrailer = "Athel-Change"

// Add makes the worktree for req. The folder must be free (doc 04 preflight step 2). A branch of
// the wanted name that already exists, locally or on the remote, is reused only when every one of
// its commits carries this change's trailer; otherwise the next suffix is tried (doc 04 edge
// case "Branch name already exists remotely").
func (t Tree) Add(req Request) (Added, error) {
	if _, err := os.Lstat(req.Dir); err == nil {
		return Added{}, aerr.New(aerr.ConflictWorktree, req.Dir+" already exists").
			WithNextAction("the worktree path must be free: athel state show lists this change's worktrees")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Added{}, aerr.Wrap(aerr.StateIO, "check "+req.Dir, err)
	}
	if _, err := t.Protect(req.Cache, req.Prefix); err != nil {
		return Added{}, err
	}
	head, err := t.baseHead(req)
	if err != nil {
		return Added{}, err
	}
	branch, reuse, err := t.pick(req)
	if err != nil {
		return Added{}, err
	}
	args := []string{"worktree", "add", "--quiet", req.Dir, branch}
	if !reuse {
		args = []string{"worktree", "add", "--quiet", "--no-track", "-b", branch, req.Dir, "refs/heads/" + req.Base}
	}
	if _, err := t.out(req.Cache, args...); err != nil {
		return Added{}, err
	}
	return Added{Branch: branch, BaseHead: head, Reused: reuse}, nil
}

// baseHead is the commit of the base branch in the cache; a repo without that branch needs its
// own base_branch in git.yaml.
func (t Tree) baseHead(req Request) (string, error) {
	args := []string{"rev-parse", "--verify", "--quiet", "refs/heads/" + req.Base}
	code, out, err := t.Git(req.Cache, args...)
	switch {
	case err == nil && code == 0:
		return strings.TrimSpace(out), nil
	case err == nil && code == 1:
		return "", aerr.New(aerr.StateNotFound, "the repo has no base branch "+req.Base).
			WithNextAction("set this repo's base_branch in .athel/config/git.yaml")
	}
	return "", gitFailure(args, out, err)
}

// pick chooses the branch: the first free name, or an earlier run's branch of this change.
func (t Tree) pick(req Request) (string, bool, error) {
	for i := 1; i <= maxSuffix; i++ {
		name := req.Branch
		if i > 1 {
			name += "-" + strconv.Itoa(i)
		}
		exists, err := t.exists(req.Cache, name)
		if err != nil || !exists {
			return name, false, err
		}
		ours, err := t.ours(req, name)
		if err != nil || ours {
			return name, ours, err
		}
	}
	return "", false, aerr.New(aerr.ConflictWorktree, "branches "+req.Branch+" to "+req.Branch+"-"+strconv.Itoa(maxSuffix)+" all belong to someone else").
		WithNextAction("ask the developer which of those branches can be deleted, or change git.yaml branch_name")
}

// exists reports whether branch exists in the cache or on the remote; a remote-only branch is
// fetched, so its commits can be checked.
func (t Tree) exists(cache, branch string) (bool, error) {
	ref := "refs/heads/" + branch
	if code, _, err := t.Git(cache, "rev-parse", "--verify", "--quiet", ref); err == nil && code == 0 {
		return true, nil
	}
	out, err := t.out(cache, "ls-remote", "--heads", "origin", ref)
	if err != nil || out == "" {
		return false, err
	}
	_, err = t.out(cache, "fetch", "--quiet", "origin", "+"+ref+":"+ref)
	return err == nil, err
}

// ours reports whether every commit on branch since base carries this change's trailer.
func (t Tree) ours(req Request, branch string) (bool, error) {
	out, err := t.out(req.Cache, "log", "--format=%(trailers:key="+changeTrailer+",valueonly,separator=%x2C)",
		"refs/heads/"+req.Base+"..refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != req.Change {
			return false, nil
		}
	}
	return out != "", nil
}

// Protect keeps the cache's fetch away from Athel's branches. M2's cache clones fetch with
// --prune and a refspec mirroring every remote branch, which would delete or overwrite a branch
// Athel has not pushed yet; a negative refspec for the prefix stops that. It reports whether it
// changed the cache's config. An empty prefix protects nothing.
func (t Tree) Protect(cache, prefix string) (bool, error) {
	if prefix == "" {
		return false, nil
	}
	neg := "^refs/heads/" + prefix + "*"
	out, err := t.out(cache, "config", "--get-all", "remote.origin.fetch")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == neg {
			return false, nil
		}
	}
	_, err = t.out(cache, "config", "--add", "remote.origin.fetch", neg)
	return err == nil, err
}

// out runs git and returns its trimmed output; a failure is E-TOOL-GIT with git's own message.
func (t Tree) out(dir string, args ...string) (string, error) {
	code, out, err := t.Git(dir, args...)
	if err == nil && code == 0 {
		return strings.TrimSpace(out), nil
	}
	return "", gitFailure(args, out, err)
}

func gitFailure(args []string, out string, err error) error {
	detail := strings.TrimSpace(out)
	if err != nil {
		detail = err.Error()
	}
	return aerr.New(aerr.ToolGit, "git "+strings.Join(args[:min(2, len(args))], " ")+": "+detail).
		WithNextAction("check the repository and the developer's git access, then retry")
}
