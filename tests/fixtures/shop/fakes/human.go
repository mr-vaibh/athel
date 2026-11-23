package fakes

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The human's side of the fake GitHub, for M5's grader and scenarios: a reviewer's decision, a CI
// result, and a merge. These are Go calls, never gh commands, so nothing Athel runs can reach
// them (FR-GIT-007).

// SetReview records a reviewer's decision (APPROVED, CHANGES_REQUESTED or REVIEW_REQUIRED).
func (s Shim) SetReview(repo string, number int, decision string) error {
	return s.prChange(repo, strconv.Itoa(number), func(p *storedPR) error {
		p.ReviewDecision = decision
		return nil
	})
}

// SetChecks records the PR's CI as one finished check with conclusion (SUCCESS or FAILURE).
func (s Shim) SetChecks(repo string, number int, conclusion string) error {
	return s.prChange(repo, strconv.Itoa(number), func(p *storedPR) error {
		p.Checks = []prCheck{{Name: "ci", Status: "COMPLETED", Conclusion: conclusion}}
		return nil
	})
}

// Merge merges an open, ready PR into its base in the bare repository acting as GitHub, with a
// merge commit as a human would, and marks it merged at the given time.
func (s Shim) Merge(repo string, number int, bare, at string) error {
	return s.prChange(repo, strconv.Itoa(number), func(p *storedPR) error {
		if p.State != "open" || p.Draft {
			return fmt.Errorf("%s#%d is %s (draft %v); only an open, ready PR merges", repo, number, p.State, p.Draft)
		}
		work, err := os.MkdirTemp("", "fake-merge-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(work) }()
		steps := [][]string{
			{"clone", "-q", "--branch", p.Base, bare, work},
			{"-C", work, "merge", "-q", "--no-ff", "-m", fmt.Sprintf("Merge pull request #%d from %s", number, p.Head), "origin/" + p.Head},
			{"-C", work, "push", "-q", "origin", p.Base},
		}
		for _, st := range steps {
			if err := humanGit(at, st...); err != nil {
				return err
			}
		}
		p.State, p.MergedAt = "merged", at
		return nil
	})
}

// humanGit runs git as the merging human, with a fixed identity and date.
func humanGit(at string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-c", "user.name=Reviewer", "-c", "user.email=reviewer@shop.example", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git with the fixture's own arguments
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at, "GIT_CONFIG_GLOBAL="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.Join(fmt.Errorf("git %s: %w", strings.Join(args, " "), err), errors.New(strings.TrimSpace(string(out))))
	}
	return nil
}
