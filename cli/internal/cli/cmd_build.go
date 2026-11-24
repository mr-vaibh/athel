package cli

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/review"
	specpkg "github.com/mr-vaibh/athel/cli/internal/spec"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Doc 06's build loop, the CLI's part: which task may start (FR-BUILD-001), the exact inputs of
// the developer and of the reviewer (FR-BUILD-002/006), and when a task is done (FR-BUILD-007/009).

const manifestSchema = "athel/build-context@1"

type buildStatus struct {
	Ready      []string     `json:"ready"`
	InProgress []string     `json:"in_progress"`
	Done       []string     `json:"done"`
	Tasks      []build.Task `json:"tasks"`
	// Next is the task to work on now, "" when nothing can go on; Blocked are the tasks open
	// cards hold, or ["build"] when a card holds the whole build (M5-plan G12).
	Next    string   `json:"next"`
	Blocked []string `json:"blocked"`
}

// cmdBuildNext lists the tasks that may start now and where every task stands.
func cmdBuildNext(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("build next"), args, 1)
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
	p, ok := s.ApprovedPlan(c)
	if !ok {
		return nil, aerr.New(aerr.StateInvalid, c.ID+" has no approved plan")
	}
	blocked, err := r.blockedTasks(c.ID)
	if err != nil {
		return nil, err
	}
	out := buildStatus{Ready: append([]string{}, build.Ready(p, c.Build)...), InProgress: []string{}, Done: []string{}, Tasks: append([]build.Task{}, c.Build...),
		Blocked: []string{}}
	for id := range blocked {
		out.Blocked = append(out.Blocked, id)
	}
	sort.Strings(out.Blocked)
	if !blocked["build"] {
		out.Next = build.Next(p, c.Build, blocked)
	}
	for _, t := range c.Build {
		switch t.Status {
		case build.Done:
			out.Done = append(out.Done, t.ID)
		case build.InProgress:
			out.InProgress = append(out.InProgress, t.ID)
		}
	}
	return out, nil
}

// manifest is the exact input set of one agent for one task: the developer's (FR-BUILD-002) or
// the reviewer's, which never holds the developer's transcript or notes (FR-BUILD-006).
type manifest struct {
	Schema       string                `yaml:"schema" json:"schema"`
	Change       string                `yaml:"change" json:"change"`
	Task         string                `yaml:"task" json:"task"`
	Role         string                `yaml:"role" json:"role"`
	TaskSpec     plan.Task             `yaml:"task_spec" json:"task_spec"`
	Requirements []specpkg.Requirement `yaml:"requirements" json:"requirements"`
	Criteria     []specpkg.Criterion   `yaml:"acceptance_criteria" json:"acceptance_criteria"`
	Conventions  string                `yaml:"conventions" json:"conventions"`
	Decisions    []string              `yaml:"decisions" json:"decisions"`
	Lessons      []string              `yaml:"lessons" json:"lessons"`
	Files        []string              `yaml:"files,omitempty" json:"files,omitempty"`
	OpenFindings []review.Finding      `yaml:"open_findings,omitempty" json:"open_findings,omitempty"`
	Diff         string                `yaml:"diff,omitempty" json:"diff,omitempty"`
	Rounds       []string              `yaml:"rounds,omitempty" json:"rounds,omitempty"`
}

// baseManifest holds what both roles get: the task, its slice of the spec, the conventions, the
// decisions it relies on, and lessons (none until M6, M4-plan G10).
func (r *runCtx) baseManifest(g gateTask, role string) (*manifest, error) {
	specs, err := r.loadSpecs(g.s, g.c.ID)
	if err != nil {
		return nil, err
	}
	m := &manifest{Schema: manifestSchema, Change: g.c.ID, Task: g.task.ID, Role: role, TaskSpec: g.task,
		Requirements: []specpkg.Requirement{}, Criteria: []specpkg.Criterion{}, Decisions: []string{}, Lessons: []string{}}
	reqs := map[string]bool{}
	for _, sp := range specs {
		for _, ac := range sp.Criteria {
			if slices.Contains(g.task.Covers, ac.ID) {
				m.Criteria = append(m.Criteria, ac)
				for _, id := range ac.Req {
					reqs[id] = true
				}
			}
		}
	}
	cited := map[string]bool{}
	for _, sp := range specs {
		for _, rq := range sp.Requirements {
			if reqs[rq.ID] {
				m.Requirements = append(m.Requirements, rq)
				for _, d := range rq.Decisions {
					cited[d] = true
				}
			}
		}
	}
	if m.Decisions, err = r.taskDecisions(g, cited); err != nil {
		return nil, err
	}
	w, _ := repoWork(g.c, g.task.Repo) // gateTask found it
	conv, err := r.conventionsOf(w.Remote)
	if err != nil {
		return nil, err
	}
	m.Conventions = conv.Dir + "/conventions.md"
	return m, nil
}

// taskDecisions are the decisions a task relies on: those its requirements cite, and cards that
// block it, each with its answer.
func (r *runCtx) taskDecisions(g gateTask, cited map[string]bool) ([]string, error) {
	f, err := r.readDecisions(g.c.ID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range f.Decisions {
		switch {
		case e.Card != nil && (cited[e.Card.ID] || slices.Contains(e.Card.Blocks, g.task.ID)):
			answer := "unanswered"
			if e.Card.Answer != nil {
				answer = strings.TrimSpace(e.Card.Answer.Option + " " + e.Card.Answer.Text)
			}
			out = append(out, e.Card.ID+": "+e.Card.Question+" → "+answer)
		case e.Decided != nil && cited[e.Decided.ID]:
			out = append(out, e.Decided.ID+": "+e.Decided.Question+" → "+e.Decided.Choice)
		}
	}
	return out, nil
}

// cmdBuildContext writes the developer's manifest: the base, the task's files, and the open
// findings to fix.
func cmdBuildContext(r *runCtx, args []string) (any, error) {
	g, err := r.gateTask(args, "build context")
	if err != nil {
		return nil, err
	}
	m, err := r.baseManifest(g, "developer")
	if err != nil {
		return nil, err
	}
	files, err := r.gitOut(g.dir, "ls-files")
	if err != nil {
		return nil, err
	}
	m.Files = []string{}
	for _, f := range strings.Fields(files) {
		if slices.ContainsFunc(g.task.Paths, func(p string) bool { return plan.MatchGlob(p, f) }) {
			m.Files = append(m.Files, f)
		}
	}
	rd, err := r.latestRound(g)
	if err != nil {
		return nil, err
	}
	if rd != nil {
		for _, f := range rd.Findings {
			if f.Status == review.Open || f.Status == review.Disputed {
				m.OpenFindings = append(m.OpenFindings, f)
			}
		}
	}
	return m, r.saveManifest(g, "build/"+g.task.ID+"/manifest.yaml", m)
}

// cmdReviewContext writes the reviewer's manifest: the base, the task's diff since its baseline,
// and the earlier rounds. Never the developer's transcript or notes (FR-BUILD-006).
func cmdReviewContext(r *runCtx, args []string) (any, error) {
	g, err := r.gateTask(args, "review context")
	if err != nil {
		return nil, err
	}
	m, err := r.baseManifest(g, "reviewer")
	if err != nil {
		return nil, err
	}
	from := g.base
	if f, err := r.loadGates(g); err == nil && f.Baseline != nil {
		from = f.Baseline.Head
	}
	diff, err := r.gitOut(g.dir, "diff", "--no-color", "--no-ext-diff", from)
	if err != nil {
		return nil, err
	}
	m.Diff = "reviews/" + g.task.ID + "/diff.patch"
	if _, err := g.s.SaveChangeFile(g.c.ID, g.session, m.Diff, []byte(diff)); err != nil {
		return nil, err
	}
	rd, err := r.latestRound(g)
	if err != nil {
		return nil, err
	}
	m.Rounds = []string{}
	if rd != nil {
		for n := 1; n <= rd.Round; n++ {
			m.Rounds = append(m.Rounds, "reviews/"+g.task.ID+"/round-"+itoa(n)+".yaml")
		}
	}
	return m, r.saveManifest(g, "reviews/"+g.task.ID+"/context.yaml", m)
}

func (r *runCtx) saveManifest(g gateTask, name string, m *manifest) error {
	b, err := yaml.Marshal(m)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "encode the manifest", err)
	}
	if err := r.registry().ValidateYAML(manifestSchema, b); err != nil {
		return err
	}
	_, err = g.s.SaveChangeFile(g.c.ID, g.session, name, b)
	return err
}

// cmdBuildDone marks a task done: its latest full gate run is green at the worktree's head, its
// review is done, and no unanswered card blocks it (FR-BUILD-007/009).
func cmdBuildDone(r *runCtx, args []string) (any, error) {
	g, err := r.planTask(args, "build done", true)
	if err != nil {
		return nil, err
	}
	if len(g.task.Paths) == 0 {
		return r.doneWithoutCode(g)
	}
	if g, err = r.withWorktree(g); err != nil {
		return nil, err
	}
	head, err := r.gitOut(g.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	head = strings.TrimSpace(head)
	problems, from, err := r.notDone(g, head)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, aerr.New(aerr.GateRed, "task "+g.task.ID+" is not done: "+strings.Join(problems, "; ")).
			WithNextAction("fix what blocks it, run athel gate run --stage full, then try again")
	}
	commits, err := r.gitOut(g.dir, "rev-list", "--reverse", "--no-merges", from+"..HEAD")
	if err != nil {
		return nil, err
	}
	return r.finishTask(g, strings.Fields(commits))
}

// doneWithoutCode finishes a task with no paths: it changes no code, so it has no worktree,
// gates, review or reports, only its cards (ADR 027).
func (r *runCtx) doneWithoutCode(g gateTask) (any, error) {
	problems, err := r.blockingCards(g)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, aerr.New(aerr.GateRed, "task "+g.task.ID+" is not done: "+strings.Join(problems, "; ")).
			WithNextAction("the developer answers the card with /athel:decide, then try again")
	}
	return r.finishTask(g, []string{})
}

func (r *runCtx) finishTask(g gateTask, commits []string) (any, error) {
	c, err := g.s.UpdateBuild(g.c.ID, g.session, func(_ *plan.Plan, recs []build.Task) ([]build.Task, error) {
		return build.Finish(recs, g.task.ID, commits, clock.Format(r.env.Clock.Now()))
	})
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(c.Build, func(t build.Task) bool { return t.ID == g.task.ID })
	return c.Build[i], nil
}

// notDone lists why a task that changes code is not done yet, and the baseline head its commits
// start from.
func (r *runCtx) notDone(g gateTask, head string) ([]string, string, error) {
	f, err := r.loadGates(g)
	if err != nil {
		return nil, "", err
	}
	problems := gateProblems(f, head)
	from := g.base
	if f.Baseline != nil {
		from = f.Baseline.Head
	}
	rd, err := r.latestRound(g)
	if err != nil {
		return nil, "", err
	}
	if rd == nil || !rd.Done() {
		problems = append(problems, "the review is not done")
	}
	missing, err := r.unfolded(g, rd)
	if err != nil {
		return nil, "", err
	}
	if len(missing) > 0 {
		problems = append(problems, "QA or security findings are not in a review round yet: "+strings.Join(missing, ", "))
	}
	problems = append(problems, reportsBehind(g, rd)...)
	cards, err := r.blockingCards(g)
	if err != nil {
		return nil, "", err
	}
	problems = append(problems, cards...)
	sort.Strings(problems)
	return problems, from, nil
}

// blockingCards are the unanswered cards that block the task or the build step.
// blockedTasks are the task IDs open cards block, and "build" when one blocks the whole build.
func (r *runCtx) blockedTasks(id string) (map[string]bool, error) {
	dec, err := r.readDecisions(id)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, card := range dec.Open("") {
		for _, b := range card.Blocks {
			if b == "build" || strings.HasPrefix(b, "T-") {
				out[b] = true
			}
		}
	}
	return out, nil
}

// cmdBuildPark ends the build step on a task an open card blocks, without blocking the change,
// so the build goes on with tasks the card does not hold, in other repos too (doc 08: work
// continues on unblocked tasks; M5-plan G12). The task stays in progress and comes back first
// once the card is answered.
func cmdBuildPark(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("build park"), args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	blocked, err := r.blockedTasks(pos[0])
	if err != nil {
		return nil, err
	}
	if !blocked[pos[1]] {
		return nil, aerr.New(aerr.StateInvalid, pos[1]+" is not held by an open card; only a task waiting on the developer is parked").
			WithNextAction("finish the task, or end the step blocked on a card that holds the build")
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	if c.Step == nil || c.Step.Status != state.StepRunning || c.Step.Name != "build" || c.Step.Task == nil || *c.Step.Task != pos[1] {
		return nil, aerr.New(aerr.StateInvalid, "the build step of "+c.ID+" is not running on "+pos[1])
	}
	if c, err = s.ExitStep(c.ID, session, "build", state.StepOK, pos[1]+" parked: it waits on a card"); err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}

func (r *runCtx) blockingCards(g gateTask) ([]string, error) {
	dec, err := r.readDecisions(g.c.ID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, card := range dec.Open("") {
		if slices.Contains(card.Blocks, g.task.ID) || slices.Contains(card.Blocks, "build") {
			out = append(out, card.ID+" is unanswered and blocks it")
		}
	}
	return out, nil
}

// reportsBehind: QA and security check every round, like the reviewer (doc 06 step 5;
// FR-BUILD-004/005), so each has a report for each review round.
func reportsBehind(g gateTask, rd *review.Round) []string {
	if rd == nil {
		return nil
	}
	var out []string
	for _, k := range []struct{ prefix, who string }{{"qa", "QA"}, {"security", "security"}} {
		if n := nextReport(g, "reviews", k.prefix) - 1; n < rd.Round {
			out = append(out, fmt.Sprintf("%s has reported for %d of %d review rounds (athel %s save)", k.who, n, rd.Round, k.prefix))
		}
	}
	return out
}

// gateProblems is why the gates do not let a task be done: no baseline, or no green full run of
// the current commit.
func gateProblems(f *gateFile, head string) []string {
	last := -1
	for i, run := range f.Runs {
		if run.Stage == gates.Full {
			last = i
		}
	}
	switch {
	case f.Baseline == nil:
		return []string{"no gate baseline"}
	case last < 0:
		return []string{"no full gate run"}
	case f.Runs[last].Head != head:
		return []string{"the last full gate run is not of the current commit"}
	case !f.Runs[last].Green:
		return []string{"the last full gate run is red"}
	}
	return nil
}

// startTask records a task as started when its build step is entered (FR-BUILD-001).
func startTask(s *state.Store, id, session, task, at string) error {
	_, err := s.UpdateBuild(id, session, func(p *plan.Plan, recs []build.Task) ([]build.Task, error) {
		return build.Start(p, recs, task, at)
	})
	return err
}
