package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/blast"
	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/decisions"
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
	out, err := r.gh("pr", "create", "--repo", ghRepo(w.Remote), "--head", w.Branch, "--base", w.Base,
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
	// Editing the body pushes nothing, so it is allowed until review: /athel:ship refreshes it with
	// the final gates, waivers and findings before marking the PR ready.
	c, w, err := r.repoTarget(s, pos[0], pos[1], state.Building, state.ReadyForPR, state.InReview)
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
	if _, err := r.gh("pr", "edit", path.Base(*w.PR), "--repo", ghRepo(w.Remote), "--title", d.Title, "--body", d.Body); err != nil {
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
	allowed, err := r.sizeAllowed(c.ID, w.Name)
	if err != nil || d.Lines <= max(d.Max, allowed) {
		return d, err
	}
	if d.Split, err = r.taskLines(s, c, w, pol.Git.PR.ExcludeFromSize, d.Max); err != nil {
		return d, err
	}
	var groups []string
	for i, g := range d.Split {
		groups = append(groups, fmt.Sprintf("PR %d: %s (%d lines)", i+1, strings.Join(g.Tasks, ", "), g.Lines))
	}
	card, err := r.raiseOnce(c.ID, sizeCard(w.Name, d.Lines, d.Max, d.Split))
	if err != nil {
		return d, err
	}
	return d, aerr.New(aerr.GatePRSize, fmt.Sprintf("the diff is %d changed lines; git.yaml allows %d per PR. Proposed split: %s",
		d.Lines, d.Max, strings.Join(groups, "; "))).
		WithNextAction("the developer answers card " + card + " with /athel:decide: raise this PR's limit, or change the plan into these PRs")
}

// sizeOption is the label of the size card's "raise" option; sizeAllowed reads the number back.
const sizeOption = "Allow up to %d changed lines in this PR"

// sizeCard asks the developer about a PR over git.yaml's limit (FR-GIT-009; ADR 028). Athel opens
// one PR per repo per change, so raising the limit is recommended (ADR 032); re-planning, the safe
// default, moves part of the work to a follow-up change.
func sizeCard(repo string, lines, limit int, split []pr.Group) decisions.Draft {
	allow := (lines*5/4 + 49) / 50 * 50 // room for fix rounds, in steps of 50
	rec := decisions.Recommendation{Option: "a", Reason: "Athel opens one PR per repo per change, so a split means moving tasks to a follow-up change"}
	if slices.ContainsFunc(split, func(g pr.Group) bool { return g.Lines > limit }) {
		rec.Reason = "one task alone is over the limit, so no split of the plan brings the PR under it"
	}
	return decisions.Draft{
		Rule: "A6", Step: "build", Question: "The " + repo + " PR is over git.yaml's limit of " + strconv.Itoa(limit) + " changed lines. How should Athel go on?",
		Context: fmt.Sprintf("FR-GIT-009: a PR is at most %d changed lines, without generated files and lockfiles. This one is %d.", limit, lines),
		Options: []decisions.Option{
			{Key: "a", Label: fmt.Sprintf(sizeOption, allow), Consequence: "Athel opens the PR as it is; reviewers get a larger diff"},
			{Key: "b", Label: "Re-plan this change with a smaller scope", Consequence: "the change goes back to planning; the plan moves part of the work to a follow-up change and needs your approval again before the PR opens"},
		},
		Recommendation: rec, Default: "b", DefaultSafe: true, Blocks: []string{"build"},
	}
}

// sizeAllowed is the PR size the developer allowed for repo by answering its size card with
// "a", or 0.
func (r *runCtx) sizeAllowed(id, repo string) (int, error) {
	f, err := r.readDecisions(id)
	if err != nil {
		return 0, err
	}
	q := "The " + repo + " PR is over git.yaml's limit of "
	allowed := 0
	for _, e := range f.Decisions {
		c := e.Card
		if c == nil || c.Answer == nil || c.Answer.Option != "a" || !strings.HasPrefix(c.Question, q) {
			continue
		}
		for _, o := range c.Options {
			var n int
			if _, err := fmt.Sscanf(o.Label, sizeOption, &n); err == nil && o.Key == "a" {
				allowed = max(allowed, n)
			}
		}
	}
	return allowed, nil
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
	if c.Parent != nil { // a side-finding fix links its parent's PRs (FR-FIND-004)
		if parent, err := s.Get(*c.Parent); err == nil {
			for _, o := range parent.Repos {
				if o.PR != nil {
					in.Linked = append(in.Linked, pr.Linked{Repo: "parent " + parent.ID + " " + o.Name, PR: *o.PR})
				}
			}
		}
	}
	in.Rollback, in.Waivers, in.Actions = planNotes(s, c, w.Name)
	if err := r.prFindingLists(s, c, w.Name, &in); err != nil {
		return prDraft{}, err
	}
	excluded := size.Excluded
	if excluded == nil {
		excluded = []string{}
	}
	return prDraft{Title: pr.Title(w.Name, c.Title), Body: pr.Body(in), Lines: size.Lines, Max: pol.Git.PR.MaxChangedLines, Excluded: excluded}, nil
}

// planNotes are the rollback steps, the waivers and the shared actions of the plan's tasks in repo.
// A repo the plan writes nothing in gets no PR, so its tasks' shared actions go to every PR.
func planNotes(s *state.Store, c *state.Change, repo string) ([]string, []string, []string) {
	var rollback, waivers, actions []string
	p, ok := s.ApprovedPlan(c)
	if !ok {
		return nil, nil, nil
	}
	for _, t := range p.Tasks {
		if t.Repo != repo {
			if !p.WritesIn(t.Repo) {
				for _, a := range t.EnvActions {
					actions = append(actions, t.ID+" ("+t.Repo+"): "+a)
				}
			}
			continue
		}
		if t.Rollback != "" {
			rollback = append(rollback, t.ID+": "+t.Rollback)
		}
		for _, a := range t.EnvActions {
			actions = append(actions, t.ID+": "+a)
		}
		for _, wv := range c.Waivers {
			if wv.Task == t.ID {
				waivers = append(waivers, wv.Task+" "+wv.Subject+" until "+wv.Until+" by "+wv.By+": "+wv.Reason)
			}
		}
	}
	return rollback, waivers, actions
}

// prFindingLists fills the side findings and the open minor review findings the PR lists.
func (r *runCtx) prFindingLists(s *state.Store, c *state.Change, repo string, in *pr.BodyInput) error {
	var err error
	if in.Findings, err = r.prFindings(s, c.ID); err != nil {
		return err
	}
	in.Minor, err = r.prMinor(s, c, repo)
	return err
}

// prMinor lists the open minor and nit review findings of the plan's tasks in repo, from each
// task's latest round: they never block, so the PR lists them (doc 06 severity table).
func (r *runCtx) prMinor(s *state.Store, c *state.Change, repo string) ([]string, error) {
	p, ok := s.ApprovedPlan(c)
	if !ok {
		return nil, nil
	}
	var out []string
	for _, t := range p.Tasks {
		if t.Repo != repo {
			continue
		}
		rd, err := r.latestRound(gateTask{s: s, c: c, task: t})
		if err != nil {
			return nil, err
		}
		if rd == nil {
			continue
		}
		for _, f := range rd.Unresolved() {
			out = append(out, t.ID+" "+f.ID+" ("+f.Severity+", "+f.Status+") "+f.Location+": "+f.Finding)
		}
	}
	return out, nil
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

// readyGuard: a change is ready for PR when every plan task is done and every repo's branch is
// pushed, lint-clean and has its draft PR (doc 06 build loop step 6; FR-GIT-007).
func (r *runCtx) readyGuard(s *state.Store, id string) error {
	c, err := s.Get(id)
	if err != nil {
		return err
	}
	p, ok := s.ApprovedPlan(c)
	if !ok {
		return guardError(id, state.ReadyForPR, []string{"the plan is not approved"})
	}
	var problems []string
	if !build.AllDone(p, c.Build) {
		problems = append(problems, "not every plan task is done (athel build next)")
	}
	for _, w := range c.Repos {
		switch {
		case !w.Pushed:
			problems = append(problems, w.Name+" is not pushed (athel git push "+id+" "+w.Name+")")
		case w.PR == nil:
			problems = append(problems, w.Name+" has no draft PR (athel pr open "+id+" "+w.Name+")")
		}
		if _, err := r.lintBranch(s, c, w); err != nil {
			problems = append(problems, w.Name+": "+err.Error())
		}
		problems = append(problems, r.blastDrift(s, c, w)...)
	}
	return guardError(id, state.ReadyForPR, problems)
}

// blastDrift compares the repo's real diff, base to the worktree's head, with the planned blast
// radius: an impact the plan does not name blocks the PR (FR-BLAST-001/005).
func (r *runCtx) blastDrift(s *state.Store, c *state.Change, w state.RepoWork) []string {
	planned, err := r.plannedReport(s, c.ID)
	if err != nil {
		return []string{"the planned blast radius: " + err.Error()}
	}
	head, err := r.gitOut(filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree)), "rev-parse", "HEAD")
	if err != nil {
		return []string{w.Name + ": " + err.Error()}
	}
	_, surface, err := r.diffSurface(w.Remote, w.BaseHead, strings.TrimSpace(head))
	if err != nil {
		return []string{w.Name + ": " + err.Error()}
	}
	if drift := blast.Drift(surface, planned); len(drift) > 0 {
		return []string{w.Name + " changes what the planned blast radius does not name (" + strings.Join(drift, "; ") +
			"): update the spec, the blast radius and the plan, and get it approved again (FR-BLAST-005)"}
	}
	return nil
}

// cmdPRReady marks a draft PR ready for review once the change is ready_for_pr and the remote
// branch is the worktree's head: every gate has passed (FR-GIT-007). Athel never merges.
func cmdPRReady(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("pr ready"), args, 2)
	if err != nil {
		return nil, err
	}
	s, _, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	w, ok := repoWork(c, pos[1])
	switch {
	case c.State != state.ReadyForPR && c.State != state.InReview:
		return nil, aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; a PR is marked ready only when the change is ready_for_pr").
			WithNextAction("finish every task, then athel state transition " + c.ID + " --to ready_for_pr")
	case !ok || w.PR == nil:
		return nil, aerr.New(aerr.StateNotFound, c.ID+" has no PR in "+pos[1])
	}
	dir := filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree))
	local, err := r.gitOut(dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	remote, err := r.gitOut(dir, "ls-remote", "--heads", "origin", "refs/heads/"+w.Branch)
	if err != nil {
		return nil, err
	}
	if f := strings.Fields(remote); len(f) == 0 || f[0] != strings.TrimSpace(local) {
		return nil, aerr.New(aerr.StateInvalid, w.Branch+" on the remote is not the worktree's head").
			WithNextAction("athel git push " + c.ID + " " + w.Name)
	}
	if _, err := r.gh("pr", "ready", path.Base(*w.PR), "--repo", ghRepo(w.Remote)); err != nil {
		return nil, err
	}
	return prOpened{URL: *w.PR}, nil
}
