package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/quality"
	"github.com/mr-vaibh/athel/cli/internal/review"
)

// Doc 06's iterative review (FR-BUILD-007/008/010): the reviewer saves rounds, the developer
// fixes or disputes findings, and the review gate passes only when nothing blocks.

func cmdReviewTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "review template", string(review.Template))
}

// cmdReviewRules lists the code quality standard's rule IDs that findings cite (doc 06).
func cmdReviewRules(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("review rules"), args, 0); err != nil {
		return nil, err
	}
	return quality.Rules, nil
}

// latestRound reads a task's newest round, or nil before the first.
func (r *runCtx) latestRound(g gateTask) (*review.Round, error) {
	dir := filepath.Join(g.s.WS.ChangeDir(g.c.ID), "reviews", g.task.ID)
	for n := review.MaxRounds; n >= 1; n-- {
		b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("round-%d.yaml", n))) //nolint:gosec // a round file of the change folder; the task ID was checked against the approved plan
		if err != nil {
			continue
		}
		if err := r.registry().ValidateYAML(review.SchemaID, b); err != nil {
			return nil, err
		}
		var rd review.Round
		if err := yaml.Unmarshal(b, &rd); err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, "round "+fmt.Sprint(n), err)
		}
		return &rd, nil
	}
	return nil, nil
}

func (r *runCtx) saveRound(g gateTask, rd *review.Round) error {
	b, err := yaml.Marshal(rd)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "encode the review round", err)
	}
	if err := r.registry().ValidateYAML(review.SchemaID, b); err != nil {
		return err
	}
	_, err = g.s.SaveChangeFile(g.c.ID, g.session, fmt.Sprintf("reviews/%s/round-%d.yaml", g.task.ID, rd.Round), b)
	return err
}

// cmdReviewSave saves the reviewer's next round of a task.
func cmdReviewSave(r *runCtx, args []string) (any, error) {
	if len(args) != 3 {
		return nil, usagef("usage: athel review save CHG-NNN T-NN -")
	}
	data, err := r.readBlock(args[2])
	if err != nil {
		return nil, err
	}
	g, err := r.gateTask(args[:2], "review save")
	if err != nil {
		return nil, err
	}
	var sub review.Submission
	if err := decodeStrict(data, &sub, "review"); err != nil {
		return nil, draftError(err, "review template")
	}
	for _, d := range sub.New {
		if err := checkLocations(g, d.Location); err != nil {
			return nil, err
		}
	}
	prev, err := r.latestRound(g)
	if err != nil {
		return nil, err
	}
	if err := r.fixesTouchTheirFiles(g, prev, sub); err != nil {
		return nil, err
	}
	head, err := r.gitOut(g.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	next, err := review.Next(prev, sub, strings.TrimSpace(head))
	if aerr.HasCode(err, aerr.BudgetRounds) {
		if _, cerr := r.raiseOnce(g.c.ID, roundLimitCard(g.task.ID)); cerr != nil {
			return nil, cerr
		}
	}
	if err != nil {
		return nil, err
	}
	all, err := r.reportFindings(g)
	if err != nil {
		return nil, err
	}
	next.Fold(all)
	return next, r.saveRound(g, next)
}

// fixesTouchTheirFiles: the reviewer may verify a fix only if its commit is in the branch and
// changed the file the finding points at (M4-plan §10, verified needs a real fix).
func (r *runCtx) fixesTouchTheirFiles(g gateTask, prev *review.Round, sub review.Submission) error {
	if prev == nil {
		return nil
	}
	for _, res := range sub.Previous {
		i := slices.IndexFunc(prev.Findings, func(f review.Finding) bool { return f.ID == res.ID })
		if res.Status != review.Verified || i < 0 || prev.Findings[i].Status != review.Fixed {
			continue
		}
		f := prev.Findings[i]
		if _, err := r.gitOut(g.dir, "merge-base", "--is-ancestor", f.FixedIn, "HEAD"); err != nil {
			return aerr.New(aerr.StateInvalid, f.ID+"'s fix "+f.FixedIn+" is not a commit of this branch")
		}
		files, err := r.gitOut(g.dir, "diff-tree", "--root", "--no-commit-id", "-r", "--name-only", f.FixedIn)
		if err != nil {
			return err
		}
		if !slices.Contains(strings.Fields(files), review.LocationFile(f.Location)) {
			return aerr.New(aerr.StateInvalid, f.ID+"'s fix "+f.FixedIn+" does not change "+review.LocationFile(f.Location)+", where the finding is").
				WithNextAction("keep the finding open, or point it at the file the fix changed")
		}
	}
	return nil
}

// cmdReviewFix marks a finding of the latest round fixed by a commit of the branch.
func cmdReviewFix(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("review fix")
	commit := fs.String("commit", "", "the commit that fixes it (required)")
	pos, err := parse(fs, args, 3)
	if err != nil {
		return nil, err
	}
	return r.changeRound(pos, func(g gateTask, rd *review.Round) error {
		sha, err := r.gitOut(g.dir, "rev-parse", "--verify", "--quiet", *commit+"^{commit}")
		if err != nil || *commit == "" {
			return aerr.New(aerr.StateInvalid, "--commit names no commit of this worktree")
		}
		return rd.Fix(pos[2], strings.TrimSpace(sha))
	})
}

// cmdReviewDispute marks a finding disputed; a second round of dispute raises a card with both
// arguments (doc 06 round rule 3).
func cmdReviewDispute(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("review dispute")
	reason := fs.String("reason", "", "why the finding is wrong (required)")
	pos, err := parse(fs, args, 3)
	if err != nil {
		return nil, err
	}
	return r.changeRound(pos, func(g gateTask, rd *review.Round) error {
		if err := rd.Dispute(pos[2], *reason); err != nil {
			return err
		}
		if rd.Disputes(pos[2]) < 2 {
			return nil
		}
		i := slices.IndexFunc(rd.Findings, func(f review.Finding) bool { return f.ID == pos[2] })
		_, err := r.raiseOnce(g.c.ID, disputeCard(g.task.ID, rd.Findings[i]))
		return err
	})
}

func (r *runCtx) changeRound(pos []string, fn func(gateTask, *review.Round) error) (any, error) {
	g, err := r.gateTask(pos[:2], "review")
	if err != nil {
		return nil, err
	}
	rd, err := r.latestRound(g)
	if err != nil {
		return nil, err
	}
	if rd == nil {
		return nil, aerr.New(aerr.StateNotFound, "task "+g.task.ID+" has no review round yet")
	}
	if err := fn(g, rd); err != nil {
		return nil, err
	}
	return rd, r.saveRound(g, rd)
}

func cmdReviewShow(r *runCtx, args []string) (any, error) {
	g, err := r.taskFor(args, "review show", false)
	if err != nil {
		return nil, err
	}
	rd, err := r.latestRound(g)
	if err == nil && rd == nil {
		err = aerr.New(aerr.StateNotFound, "task "+g.task.ID+" has no review round yet")
	}
	return rd, err
}

// judgeReview is the review gate: the latest round approves and nothing blocks (never
// waivable; doc 06).
func (r *runCtx) judgeReview(g gateTask, o *gateOutcome) {
	rd, err := r.latestRound(g)
	var missing []string
	if err == nil {
		missing, err = r.unfolded(g, rd)
	}
	switch {
	case err != nil:
		o.Status, o.Summary = "unavailable", err.Error()
	case len(missing) > 0:
		o.Status, o.Summary = gates.Fail, "QA or security findings are not in a review round yet: "+strings.Join(missing, ", ")
		o.Details = []string{"the reviewer's next round takes them in: athel review save"}
	case rd == nil:
		o.Status, o.Summary = gates.Fail, "no review round yet"
	case !rd.Done():
		o.Status, o.Summary = gates.Fail, fmt.Sprintf("round %d is %s", rd.Round, rd.Verdict)
		for _, id := range rd.Blocking() {
			o.Details = append(o.Details, id+" blocks until verified (or, if major, waived by a human)")
		}
	}
}

func roundLimitCard(task string) decisions.Draft {
	return decisions.Draft{
		Rule: "A6", Step: "build", Question: "Task " + task + " reached " + fmt.Sprint(review.MaxRounds) + " review rounds. How should Athel go on?",
		Context: "FR-BUILD-010: after 5 rounds the fix loop stops and asks instead of burning budget.",
		Options: []decisions.Option{
			{Key: "a", Label: "Stop the task and look at it yourself", Consequence: "Athel writes nothing more for " + task + " until you answer again"},
			{Key: "b", Label: "Allow more rounds", Consequence: "the loop continues; the open findings stay as they are"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "five rounds without agreement usually means the task or the spec needs a human"},
		Default:        "a", DefaultSafe: true, Blocks: []string{task},
	}
}

func disputeCard(task string, f review.Finding) decisions.Draft {
	reviewer := f.Required
	for _, s := range f.History {
		if s.Status == review.Open && s.Note != "" {
			reviewer = s.Note
		}
	}
	return decisions.Draft{
		Rule: "A6", Step: "build", Question: "The reviewer and the developer disagree about " + f.ID + " in task " + task + ". Who is right?",
		Context: f.ID + " (" + f.Severity + ", " + f.Location + "): " + f.Finding + " Reviewer: " + reviewer + " Developer: " + f.Dispute,
		Options: []decisions.Option{
			{Key: "a", Label: "The reviewer: fix it", Consequence: "the developer makes the fix the finding asks for"},
			{Key: "b", Label: "The developer: close the finding", Consequence: "the finding is closed by your decision, recorded with the card"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "a reviewer's finding is the safer default until a human says otherwise"},
		Default:        "a", DefaultSafe: true, Blocks: []string{task},
	}
}

// identicalFailures reports whether the last three runs were red with the same red gates and
// details (FR-BUILD-010: "3 identical failures in a row").
func identicalFailures(runs []gateRun) bool {
	if len(runs) < 3 {
		return false
	}
	sig := func(run gateRun) string {
		var b strings.Builder
		for _, o := range run.Results {
			if o.Status == gates.Fail || o.Status == "unavailable" {
				fmt.Fprintf(&b, "%s|%s|%s\n", o.Gate, o.Status, strings.Join(o.Details, ";"))
			}
		}
		return b.String()
	}
	last := runs[len(runs)-3:]
	first := sig(last[0])
	return first != "" && !last[0].Green && sig(last[1]) == first && sig(last[2]) == first
}

func identicalCard(task string) decisions.Draft {
	return decisions.Draft{
		Rule: "A6", Step: "build", Question: "Task " + task + "'s gates failed the same way three times in a row. How should Athel go on?",
		Context: "FR-BUILD-010: three identical failures in a row stop the fix loop; the developer agent is not getting closer.",
		Options: []decisions.Option{
			{Key: "a", Label: "Stop the task and look at it yourself", Consequence: "Athel writes nothing more for " + task + " until you answer again"},
			{Key: "b", Label: "Try once more", Consequence: "one more fix round; the counter starts again"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: "the same failure three times means the developer agent cannot fix it alone"},
		Default:        "a", DefaultSafe: true, Blocks: []string{task},
	}
}
