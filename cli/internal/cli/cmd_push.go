package cli

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gitw"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

type pushed struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Head   string `json:"head"`
	gitw.Integration
}

// cmdGitPush is the only way Athel's work leaves the machine (ADR 026; doc 04 §Before every
// push, FR-GIT-004/006/010): re-fetch; stop if anyone else pushed to Athel's branch; bring the
// base in (rebase while never pushed, merge after); lint every commit; push, never forced, only
// Athel's own branch of a building change.
func cmdGitPush(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("git push"), args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, w, err := r.pushTarget(s, pos[0], pos[1])
	if err != nil {
		return nil, err
	}
	cache, err := r.cacheClone(w.Remote)
	if err != nil {
		return nil, err
	}
	tree := gitw.Tree{Git: r.networkGit}
	dir := filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree))
	if err := r.stopIfForeign(tree, c.ID, w, cache.Path, dir); err != nil {
		return nil, err
	}
	in, err := tree.Integrate(dir, w.Base, w.Pushed, r.ownedBy(s, c, w.Name))
	if err != nil {
		if len(in.Unowned) > 0 {
			if _, cerr := r.raiseOnce(c.ID, conflictCard(w.Name, in.Unowned)); cerr != nil {
				return nil, cerr
			}
		}
		return in, err
	}
	if _, err := r.lintBranch(s, c, w); err != nil {
		return nil, err
	}
	if err := tree.Push(dir, w.Branch); err != nil {
		return nil, err
	}
	if _, err := s.UpdateRepo(c.ID, session, w.Name, func(rw *state.RepoWork) { rw.Pushed = true }); err != nil {
		return nil, err
	}
	head, err := r.gitOut(dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	return pushed{Repo: w.Name, Branch: w.Branch, Head: strings.TrimSpace(head), Integration: in}, nil
}

// pushTarget is the building change and its worktree in repo, on a branch Athel may push.
func (r *runCtx) pushTarget(s *state.Store, id, repo string) (*state.Change, state.RepoWork, error) {
	c, err := s.Get(id)
	if err != nil {
		return nil, state.RepoWork{}, err
	}
	if c.State != state.Building {
		return nil, state.RepoWork{}, aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; Athel pushes only while building")
	}
	w, ok := repoWork(c, repo)
	if !ok {
		return nil, state.RepoWork{}, aerr.New(aerr.StateNotFound, c.ID+" has no worktree in "+repo).WithNextAction("athel git preflight " + c.ID + " " + repo)
	}
	return c, w, r.pushAllowed(w.Branch)
}

// pushAllowed refuses a protected branch, whatever change.yaml says (FR-GIT-004).
func (r *runCtx) pushAllowed(branch string) error {
	pol, err := r.policySet()
	if err != nil {
		return err
	}
	for _, p := range append([]string{pol.Git.BaseBranch}, pol.Git.Protected...) {
		if ok, _ := path.Match(p, branch); ok || p == branch {
			return aerr.New(aerr.PolicyForbidden, "Athel never pushes to protected branch "+branch)
		}
	}
	return nil
}

// stopIfForeign raises a card and stops when someone else pushed to Athel's branch.
func (r *runCtx) stopIfForeign(tree gitw.Tree, id string, w state.RepoWork, cache, dir string) error {
	ahead, err := tree.RemoteAhead(cache, dir, w.Branch)
	if err != nil || !ahead {
		return err
	}
	card, err := r.raiseOnce(id, foreignCard(w.Name, w.Branch))
	if err != nil {
		return err
	}
	return aerr.New(aerr.ConflictForeignPush, "someone else pushed to "+w.Branch+"; Athel never overwrites their commits").
		WithNextAction("card " + card + " asks the developer how to go on; Athel writes nothing more to this branch until then")
}

// ownedBy reports whether a path is inside the paths of a plan task in repo (FR-GIT-010).
func (r *runCtx) ownedBy(s *state.Store, c *state.Change, repo string) func(string) bool {
	var globs []string
	if p, ok := s.ApprovedPlan(c); ok {
		for _, t := range p.Tasks {
			if t.Repo == repo {
				globs = append(globs, t.Paths...)
			}
		}
	}
	return func(f string) bool {
		return slices.ContainsFunc(globs, func(g string) bool { return plan.MatchGlob(g, f) })
	}
}

// raiseOnce adds a card unless an unanswered card asks the same question; it returns its ID.
func (r *runCtx) raiseOnce(id string, d decisions.Draft) (string, error) {
	var card string
	err := r.editDecisions(id, func(f *decisions.File) error {
		for _, open := range f.Open("") {
			if open.Question == d.Question {
				card = open.ID
				return nil
			}
		}
		c, err := f.Add(d)
		if err == nil {
			card = c.ID
		}
		return err
	})
	return card, err
}

func foreignCard(repo, branch string) decisions.Draft {
	return decisions.Draft{
		Rule: "A4", Step: "build", Question: "Someone else pushed commits to Athel's branch " + branch + " in " + repo + ". How should Athel go on?",
		Context: "Doc 04: Athel stops writing to a branch someone else pushed to and never overwrites their commits.",
		Options: []decisions.Option{
			{Key: "a", Label: "Stop and talk to whoever pushed", Consequence: "Athel writes nothing more to " + branch + " until you answer again"},
			{Key: "b", Label: "Keep their commits and continue", Consequence: "Athel merges their commits into its work and pushes on top; their changes go into this PR"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "their commits may conflict with the plan, and only you know why they pushed"},
		Default:        "a", DefaultSafe: true, Blocks: []string{"build"},
	}
}

func conflictCard(repo string, files []string) decisions.Draft {
	return decisions.Draft{
		Rule: "A4", Step: "build", Question: "Merging the base into Athel's branch in " + repo + " conflicts in files the change does not own. How should Athel go on?",
		Context: "FR-GIT-010: Athel resolves conflicts only in files its plan owns. Conflicted: " + strings.Join(files, ", ") + ".",
		Options: []decisions.Option{
			{Key: "a", Label: "You resolve the conflict", Consequence: "you merge the base into the branch by hand; Athel then pushes again"},
			{Key: "b", Label: "Change the plan to own those files", Consequence: "the plan changes and needs your approval again; Athel then resolves the conflict"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "the files belong to work outside this change"},
		Default:        "a", DefaultSafe: true, Blocks: []string{"build"},
	}
}
