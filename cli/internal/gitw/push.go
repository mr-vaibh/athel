package gitw

import (
	"os"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Doc 04 §Before every push: re-fetch; abort if anyone else has pushed to Athel's branch; rebase
// on base only while the branch was never pushed, and once pushed merge base into it instead,
// since rewriting pushed history is forbidden. Conflicts are resolved only in files the change
// owns (FR-GIT-010). These steps run in Athel's worktree after the cache was fetched.

// remoteRef keeps the fetched remote copy of Athel's branch out of refs/heads, which the
// cache's fetch manages.
const remoteRef = "refs/athel/remote/"

// RemoteAhead reports whether the remote copy of branch has commits HEAD does not: someone else
// pushed to Athel's branch, and Athel must stop writing to it (doc 04 edge case).
func (t Tree) RemoteAhead(cache, dir, branch string) (bool, error) {
	out, err := t.out(cache, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	if err != nil || out == "" {
		return false, err
	}
	ref := remoteRef + branch
	// --refmap= stops git also updating refs/heads/<branch> through the cache's configured
	// refspec, which git refuses while the branch is checked out in Athel's worktree.
	if _, err := t.out(cache, "fetch", "--quiet", "--refmap=", "origin", "+refs/heads/"+branch+":"+ref); err != nil {
		return false, err
	}
	contained, err := t.ancestor(dir, ref, "HEAD")
	return !contained, err
}

// ancestor reports whether a is an ancestor of b (or equal to it).
func (t Tree) ancestor(dir, a, b string) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", a, b}
	code, out, err := t.Git(dir, args...)
	switch {
	case err == nil && code == 0:
		return true, nil
	case err == nil && code == 1:
		return false, nil
	}
	return false, gitFailure(args, out, err)
}

// Integration is how the base was brought into Athel's branch.
type Integration struct {
	Rebased   bool     `json:"rebased"`
	Merged    bool     `json:"merged"`
	Conflicts []string `json:"conflicts"`
	Unowned   []string `json:"unowned"` // conflicted files outside the plan's paths
}

// Integrate brings base into HEAD: nothing when HEAD already contains it, a rebase while the
// branch was never pushed, otherwise (or when the rebase conflicts) a merge. A merge conflict
// only in owned files is left in progress for the developer agent to resolve; any conflicted
// file it does not own aborts the merge, so a human decides. Both are E-CONFLICT-MERGE.
func (t Tree) Integrate(dir, base string, pushed bool, owned func(string) bool) (Integration, error) {
	var got Integration
	if p, err := t.out(dir, "rev-parse", "--git-path", "MERGE_HEAD"); err != nil {
		return got, err
	} else if _, err := os.Stat(p); err == nil {
		return got, aerr.New(aerr.ConflictMerge, "a merge of "+base+" is in progress").
			WithNextAction("resolve the conflicted files, commit the merge with git commit, then push again")
	}
	ref := "refs/heads/" + base
	if up, err := t.ancestor(dir, ref, "HEAD"); err != nil || up {
		return got, err
	}
	if !pushed {
		if code, _, err := t.Git(dir, "rebase", "--quiet", ref); err == nil && code == 0 {
			got.Rebased = true
			return got, nil
		}
		if _, err := t.out(dir, "rebase", "--abort"); err != nil {
			return got, err
		}
	}
	msg := "chore: merge " + base + " into this branch"
	if code, _, err := t.Git(dir, "merge", "--no-verify", "--no-edit", "--no-ff", "-m", msg, ref); err == nil && code == 0 {
		got.Merged = true
		return got, nil
	}
	return got, t.conflict(dir, base, owned, &got)
}

// conflict lists a failed merge's conflicted files and aborts it when one is not owned.
func (t Tree) conflict(dir, base string, owned func(string) bool, got *Integration) error {
	out, err := t.out(dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return err
	}
	got.Conflicts = strings.Fields(out)
	for _, f := range got.Conflicts {
		if !owned(f) {
			got.Unowned = append(got.Unowned, f)
		}
	}
	if len(got.Unowned) > 0 {
		if _, err := t.out(dir, "merge", "--abort"); err != nil {
			return err
		}
		return aerr.New(aerr.ConflictMerge, "merging "+base+" conflicts in files this change does not own: "+strings.Join(got.Unowned, ", ")).
			WithNextAction("a human resolves this conflict; Athel resolves conflicts only in files its plan owns")
	}
	return aerr.New(aerr.ConflictMerge, "merging "+base+" conflicts in "+strings.Join(got.Conflicts, ", ")).
		WithNextAction("resolve the conflicted files, commit the merge with git commit, then push again")
}

// Push pushes HEAD to Athel's branch, never forced.
func (t Tree) Push(dir, branch string) error {
	_, err := t.out(dir, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch)
	return err
}
