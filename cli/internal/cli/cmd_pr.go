package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gitlint"
	"github.com/mr-vaibh/athel/cli/internal/pr"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Athel's pull requests (doc 04 §Git policy, FR-GIT-007/009; ADR 026): opened as drafts, the
// body built from the change's files, the size checked against git.yaml's per-PR limit before
// anything reaches GitHub, and never merged. Marking a PR ready needs every gate green, so it
// comes with the build loop (M4 task 10).

type prDraft struct {
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Lines    int        `json:"lines"`
	Max      int        `json:"max_changed_lines"`
	Excluded []string   `json:"excluded"`
	Split    []pr.Group `json:"split,omitempty"`
}

type prOpened struct {
	prDraft
	URL     string `json:"url"`
	Created bool   `json:"created"`
}

func cmdPRBody(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("pr body"), args, 2)
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
		return nil, aerr.New(aerr.StateNotFound, c.ID+" has no worktree in "+pos[1])
	}
	return r.prDraft(s, c, w)
}

// cmdPROpen opens the draft PR of a pushed branch, or returns the one already open.
func cmdPROpen(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("pr open"), args, 2)
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
	if !w.Pushed {
		return nil, aerr.New(aerr.StateInvalid, w.Branch+" is not pushed yet").WithNextAction("athel git push " + c.ID + " " + w.Name)
	}
	d, err := r.sizedDraft(s, c, w)
	if err != nil {
		return d, err
	}
	if w.PR != nil {
		return prOpened{prDraft: d, URL: *w.PR}, nil
	}
	out, err := r.tool("gh", "pr", "create", "--repo", ghRepo(w.Remote), "--head", w.Branch, "--base", w.Base,
		"--title", d.Title, "--body", d.Body, "--draft")
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return nil, aerr.New(aerr.ToolUnreachable, "gh pr create printed no URL")
	}
	url := lines[len(lines)-1]
	if _, err := s.UpdateRepo(c.ID, session, w.Name, func(rw *state.RepoWork) { rw.PR = &url }); err != nil {
		return nil, err
	}
	return prOpened{prDraft: d, URL: url, Created: true}, nil
}

// cmdPRUpdate refreshes the open PR's description after more work was pushed.
func cmdPRUpdate(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("pr update"), args, 2)
	if err != nil {
		return nil, err
	}
	s, _, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, w, err := r.pushTarget(s, pos[0], pos[1])
	if err != nil {
		return nil, err
	}
	if w.PR == nil {
		return nil, aerr.New(aerr.StateInvalid, c.ID+" has no PR in "+w.Name+" yet").WithNextAction("athel pr open " + c.ID + " " + w.Name)
	}
	d, err := r.sizedDraft(s, c, w)
	if err != nil {
		return d, err
	}
	if _, err := r.tool("gh", "pr", "edit", path.Base(*w.PR), "--repo", ghRepo(w.Remote), "--title", d.Title, "--body", d.Body); err != nil {
		return nil, err
	}
	return prOpened{prDraft: d, URL: *w.PR}, nil
}

// sizedDraft is the PR draft, refused when its title breaks git.yaml or its diff is over the
// per-PR limit; then the error carries a split of the plan's tasks into PRs under the limit.
func (r *runCtx) sizedDraft(s *state.Store, c *state.Change, w state.RepoWork) (prDraft, error) {
	d, err := r.prDraft(s, c, w)
	if err != nil {
		return d, err
	}
	pol, _ := r.policySet() // prDraft loaded it
	if p := gitlint.Title(d.Title, pol.Git.Commit.Format); len(p) > 0 {
		return d, lintFailure("PR title refused", p)
	}
	if d.Lines <= d.Max {
		return d, nil
	}
	if d.Split, err = r.taskLines(s, c, w, pol.Git.PR.ExcludeFromSize, d.Max); err != nil {
		return d, err
	}
	var groups []string
	for i, g := range d.Split {
		groups = append(groups, fmt.Sprintf("PR %d: %s (%d lines)", i+1, strings.Join(g.Tasks, ", "), g.Lines))
	}
	return d, aerr.New(aerr.GatePRSize, fmt.Sprintf("the diff is %d changed lines; git.yaml allows %d per PR. Proposed split: %s",
		d.Lines, d.Max, strings.Join(groups, "; "))).
		WithNextAction("split the plan into these PRs with a plan change request, or have the limit raised at plan approval")
}

// prDraft builds the title and body from the change's files and the branch's diff.
func (r *runCtx) prDraft(s *state.Store, c *state.Change, w state.RepoWork) (prDraft, error) {
	pol, err := r.policySet()
	if err != nil {
		return prDraft{}, err
	}
	dir := filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree))
	numstat, err := r.gitOut(dir, "diff", "--numstat", "refs/heads/"+w.Base+"...HEAD")
	if err != nil {
		return prDraft{}, err
	}
	size := pr.Size(numstat, pol.Git.PR.ExcludeFromSize)
	risk, err := r.blastRisk(s, c.ID)
	if err != nil {
		return prDraft{}, err
	}
	in := pr.BodyInput{Change: c.ID, Title: c.Title, Repo: w.Name, Spec: specFor(s, c.ID, w.Name), Risk: risk, Files: size.Files}
	if c.PRD.Current != "" {
		in.PRD = "prd/" + c.PRD.Current + ".md"
	}
	if in.Decisions, err = r.prDecisions(c.ID); err != nil {
		return prDraft{}, err
	}
	for _, o := range c.Repos {
		if o.Name != w.Name && o.PR != nil {
			in.Linked = append(in.Linked, pr.Linked{Repo: o.Name, PR: *o.PR})
		}
	}
	if p, ok := s.ApprovedPlan(c); ok {
		for _, t := range p.Tasks {
			if t.Repo == w.Name && t.Rollback != "" {
				in.Rollback = append(in.Rollback, t.ID+": "+t.Rollback)
			}
		}
	}
	excluded := size.Excluded
	if excluded == nil {
		excluded = []string{}
	}
	return prDraft{Title: pr.Title(w.Name, c.Title), Body: pr.Body(in), Lines: size.Lines, Max: pol.Git.PR.MaxChangedLines, Excluded: excluded}, nil
}

// specFor is the change's spec for repo when it has one by that name, else every spec.
func specFor(s *state.Store, id, repo string) string {
	dir := filepath.Join(s.WS.ChangeDir(id), "spec")
	files, _ := filepath.Glob(filepath.Join(dir, "*.md")) // the pattern is valid
	var names []string
	for _, f := range files {
		rel := "spec/" + filepath.Base(f)
		if filepath.Base(f) == repo+".md" {
			return rel
		}
		names = append(names, rel)
	}
	return strings.Join(names, ", ")
}

// prDecisions are the answered cards and Athel's own recorded choices.
func (r *runCtx) prDecisions(id string) ([]pr.Decision, error) {
	f, err := r.readDecisions(id)
	if err != nil {
		return nil, err
	}
	var out []pr.Decision
	for _, e := range f.Decisions {
		switch {
		case e.Card != nil && e.Card.Answer != nil:
			answer := e.Card.Answer.Option
			if answer == "" {
				answer = e.Card.Answer.Text
			}
			out = append(out, pr.Decision{ID: e.Card.ID, Question: e.Card.Question, Answer: answer, By: e.Card.Answer.By})
		case e.Decided != nil:
			out = append(out, pr.Decision{ID: e.Decided.ID, Question: e.Decided.Question, Answer: e.Decided.Choice, By: "athel"})
		}
	}
	return out, nil
}

// taskLines sizes each plan task's commits on the branch, in plan order, for a split proposal.
func (r *runCtx) taskLines(s *state.Store, c *state.Change, w state.RepoWork, exclude []string, limit int) ([]pr.Group, error) {
	dir := filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree))
	shas, err := r.gitOut(dir, "rev-list", "--no-merges", "HEAD", "^"+w.BaseHead, "^refs/heads/"+w.Base)
	if err != nil {
		return nil, err
	}
	lines := map[string]int{}
	for _, sha := range strings.Fields(shas) {
		task, err := r.gitOut(dir, "log", "-1", "--format=%(trailers:key=Athel-Task,valueonly)", sha)
		if err != nil {
			return nil, err
		}
		numstat, err := r.gitOut(dir, "show", "--numstat", "--format=", sha)
		if err != nil {
			return nil, err
		}
		lines[strings.TrimSpace(task)] += pr.Size(numstat, exclude).Lines
	}
	var tasks []pr.TaskLines
	if p, ok := s.ApprovedPlan(c); ok {
		for _, t := range p.Tasks {
			if t.Repo == w.Name {
				tasks = append(tasks, pr.TaskLines{Task: t.ID, Lines: lines[t.ID]})
			}
		}
	}
	return pr.Split(tasks, limit), nil
}
