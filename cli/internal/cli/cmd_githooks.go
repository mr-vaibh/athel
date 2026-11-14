package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/gitlint"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Athel's commits are linted by git itself: every cache clone's worktrees use the commit-msg and
// pre-commit hooks in .athel/git-hooks, which call this binary (FR-GIT-005, FR-BUILD-012, doc 04
// "pre-commit check blocks it"). A hook sees the message however the agent wrote it, which a
// look at the command line cannot. Skipping the hooks is forbidden (classify), and `athel git
// lint` checks every commit again before a push.

// gitHooks are the hooks Athel installs, each calling `athel git hook <name>`.
var gitHooks = []string{"commit-msg", "pre-commit"}

// installHooks writes the hooks and points the cache clone's worktrees at them.
func (r *runCtx) installHooks(s *state.Store, cache string) error {
	if r.env.Executable == nil {
		return aerr.New(aerr.StateIO, "cannot find the athel binary for git hooks")
	}
	exe, err := r.env.Executable()
	if err != nil {
		return aerr.Wrap(aerr.StateIO, "find the athel binary for git hooks", err)
	}
	dir := filepath.Join(s.WS.Root, ".athel", "git-hooks")
	if err := fsutil.EnsureDir(dir); err != nil {
		return err
	}
	for _, name := range gitHooks {
		arg := ""
		if name == "commit-msg" {
			arg = ` "$1"`
		}
		script := "#!/bin/sh\n# Installed by Athel: lints every commit in Athel's worktrees (doc 04).\nexec " +
			shellQuote(exe) + " git hook " + name + arg + "\n"
		p := filepath.Join(dir, name)
		if err := fsutil.WriteFileAtomic(p, []byte(script)); err != nil {
			return err
		}
		if err := os.Chmod(p, 0o700); err != nil { //nolint:gosec // a git hook must be executable
			return aerr.Wrap(aerr.StateIO, "make "+p+" executable", err)
		}
	}
	_, err = r.gitOut(cache, "config", "core.hooksPath", dir)
	return err
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// worktreeAt finds the change and repo whose worktree holds the current folder.
func (r *runCtx) worktreeAt() (*state.Store, *state.Change, state.RepoWork, error) {
	s, err := r.storeFor()
	if err != nil {
		return nil, nil, state.RepoWork{}, err
	}
	rel, err := filepath.Rel(s.WS.Root, r.env.Cwd)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if err == nil && len(parts) >= 2 && parts[0] == "repos" {
		if repo, id, ok := strings.Cut(parts[1], "@"); ok {
			c, err := s.Get(id)
			if err != nil {
				return nil, nil, state.RepoWork{}, err
			}
			for _, w := range c.Repos {
				if w.Name == repo {
					return s, c, w, nil
				}
			}
		}
	}
	return nil, nil, state.RepoWork{}, aerr.New(aerr.StateNotFound, r.env.Cwd+" is not one of Athel's worktrees")
}

// lintRules are git.yaml's commit rules and the plan's tasks in this repo.
func (r *runCtx) lintRules(s *state.Store, c *state.Change, repo string) (gitlint.Rules, []string, error) {
	pol, err := r.policySet()
	if err != nil {
		return gitlint.Rules{}, nil, err
	}
	var tasks []string
	if p, ok := s.ApprovedPlan(c); ok {
		for _, t := range p.Tasks {
			if t.Repo == repo {
				tasks = append(tasks, t.ID)
			}
		}
	}
	return gitlint.Rules{Format: pol.Git.Commit.Format, Trailers: pol.Git.Commit.Trailer, MaxFiles: pol.Git.Commit.MaxFiles}, tasks, nil
}

func lintFailure(what string, problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	return aerr.New(aerr.GateCommitLint, what+": "+strings.Join(problems, "; ")).
		WithNextAction("fix the commit and commit again; git.yaml says how Athel's commits look")
}

// cmdGitHook runs as git's commit-msg or pre-commit hook in one of Athel's worktrees.
func cmdGitHook(r *runCtx, args []string) (any, error) {
	if len(args) == 0 || (args[0] != "commit-msg" && args[0] != "pre-commit") || (args[0] == "commit-msg") != (len(args) == 2) {
		return nil, usagef("usage: athel git hook commit-msg <message-file> | athel git hook pre-commit")
	}
	s, c, w, err := r.worktreeAt()
	if err != nil {
		return nil, err
	}
	rules, tasks, err := r.lintRules(s, c, w.Name)
	if err != nil {
		return nil, err
	}
	if args[0] == "commit-msg" {
		file := args[1]
		if !filepath.IsAbs(file) {
			file = filepath.Join(r.env.Cwd, file)
		}
		msg, err := os.ReadFile(file) //nolint:gosec // the message file git names for this commit
		if err != nil {
			return nil, aerr.Wrap(aerr.StateIO, "read the commit message", err)
		}
		return map[string]bool{"ok": true}, lintFailure("commit message refused", gitlint.Message(string(msg), rules, c.ID, tasks))
	}
	staged, err := r.stagedFiles(r.env.Cwd, "")
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, lintFailure("commit refused", gitlint.StagedFiles(staged, rules))
}

// stagedFiles lists what a commit adds or changes: the index of the worktree when sha is "",
// otherwise the commit sha. Sizes are the blobs', added text is from a zero-context diff.
func (r *runCtx) stagedFiles(dir, sha string) ([]gitlint.Staged, error) {
	namesArgs := []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"}
	diffArgs := []string{"diff", "--cached", "-U0", "--no-color", "--no-ext-diff"}
	blob := ":"
	if sha != "" {
		namesArgs = []string{"diff-tree", "--root", "--no-commit-id", "-r", "--name-only", "--diff-filter=ACMR", "-z", sha}
		diffArgs = []string{"show", "--format=", "-U0", "--no-color", "--no-ext-diff", sha}
		blob = sha + ":"
	}
	names, err := r.gitOut(dir, namesArgs...)
	if err != nil {
		return nil, err
	}
	diff, err := r.gitOut(dir, diffArgs...)
	if err != nil {
		return nil, err
	}
	added := addedLines(diff)
	var out []gitlint.Staged
	for _, name := range strings.Split(strings.TrimRight(names, "\x00"), "\x00") {
		if name == "" {
			continue
		}
		size, err := r.gitOut(dir, "cat-file", "-s", blob+name)
		if err != nil {
			return nil, err
		}
		n, _ := strconv.ParseInt(strings.TrimSpace(size), 10, 64) // git prints a number
		out = append(out, gitlint.Staged{Path: name, Size: n, Added: added[name]})
	}
	return out, nil
}

// addedLines groups a unified diff's added lines by file.
func addedLines(diff string) map[string]string {
	out := map[string]string{}
	file := ""
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
		case strings.HasPrefix(l, "+"):
			out[file] += l[1:] + "\n"
		}
	}
	return out
}

type lintedCommit struct {
	SHA      string   `json:"sha"`
	Subject  string   `json:"subject"`
	Problems []string `json:"problems"`
}

type lintResult struct {
	Repo    string         `json:"repo"`
	Branch  string         `json:"branch"`
	OK      bool           `json:"ok"`
	Commits []lintedCommit `json:"commits"`
}

// cmdGitLint checks every commit on Athel's branch since the base it was cut from, and the
// branch name (FR-GIT-005): the same rules as the hooks, again before anything leaves the
// machine. Merge commits are Athel's own merges of base and are not linted.
func cmdGitLint(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("git lint"), args, 2)
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
	w, ok := repoWork(c, pos[1])
	if !ok {
		return nil, aerr.New(aerr.StateNotFound, c.ID+" has no worktree in "+pos[1]).WithNextAction("athel git preflight " + c.ID + " " + pos[1])
	}
	rules, tasks, err := r.lintRules(s, c, w.Name)
	if err != nil {
		return nil, err
	}
	pol, _ := r.policySet() // lintRules loaded it
	dir := filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree))
	out := lintResult{Repo: w.Name, Branch: w.Branch, OK: true, Commits: []lintedCommit{}}
	var all []string
	all = append(all, gitlint.Branch(w.Branch, pol.Git.BranchName, c.ID)...)
	shas, err := r.gitOut(dir, "rev-list", "--reverse", "--no-merges", w.BaseHead+"..HEAD")
	if err != nil {
		return nil, err
	}
	for _, sha := range strings.Fields(shas) {
		msg, err := r.gitOut(dir, "log", "-1", "--format=%B", sha)
		if err != nil {
			return nil, err
		}
		files, err := r.stagedFiles(dir, sha)
		if err != nil {
			return nil, err
		}
		problems := append(gitlint.Message(msg, rules, c.ID, tasks), gitlint.StagedFiles(files, rules)...)
		subject, _, _ := strings.Cut(strings.TrimSpace(msg), "\n")
		out.Commits = append(out.Commits, lintedCommit{SHA: sha, Subject: subject, Problems: append([]string{}, problems...)})
		for _, p := range problems {
			all = append(all, sha[:12]+": "+p)
		}
	}
	out.OK = len(all) == 0
	if !out.OK {
		return out, lintFailure(w.Branch+" has commits git.yaml does not allow", all)
	}
	return out, nil
}

func repoWork(c *state.Change, repo string) (state.RepoWork, bool) {
	for _, w := range c.Repos {
		if w.Name == repo {
			return w, true
		}
	}
	return state.RepoWork{}, false
}
