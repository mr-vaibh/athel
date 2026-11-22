package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/preflight"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

type preflightDone struct {
	Repo     string              `json:"repo"`
	Branch   string              `json:"branch"`
	Clean    bool                `json:"clean"`
	Overlaps []preflight.Overlap `json:"overlaps"`
	Card     *string             `json:"card"`
	Report   string              `json:"report"`
}

// cmdGitPreflight is doc 04's preflight for one repo of an approved change (FR-GIT-002/003):
// the worktree from a freshly fetched cache, others' open PRs, recently pushed branches and
// recent commits to base, their overlap with the plan's paths, and, when someone else's open
// work overlaps, an A4 card that blocks the build. The report is preflight/<repo>.md. Running it
// again refreshes the report and finds the card it raised.
func cmdGitPreflight(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("git preflight"), args, 2)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	id, repo := pos[0], pos[1]
	tree, err := r.ensureWorktree(s, session, id, repo)
	if err != nil {
		return nil, err
	}
	c, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	p, _ := s.ApprovedPlan(c) // ensureWorktree checked that an approved plan writes in repo
	var globs []string
	for _, t := range p.Tasks {
		if t.Repo == repo {
			globs = append(globs, t.Paths...)
		}
	}
	work, err := r.activeWork(tree)
	if err != nil {
		return nil, err
	}
	overlaps := preflight.Overlaps(work, globs)
	card, err := r.preflightCard(id, repo, overlaps)
	if err != nil {
		return nil, err
	}
	rep := preflight.Report{Change: id, Repo: repo, Remote: tree.Remote, Base: tree.Base, BaseHead: tree.BaseHead, Branch: tree.Branch,
		At: clock.Format(r.env.Clock.Now()), PlannedPaths: globs, Work: work, Overlaps: overlaps, Card: card}
	md, err := rep.Render(r.registry())
	if err != nil {
		return nil, err
	}
	name := "preflight/" + repo + ".md"
	if _, err := s.SaveChangeFile(id, session, name, md); err != nil {
		return nil, err
	}
	out := preflightDone{Repo: repo, Branch: tree.Branch, Clean: preflight.Blocking(overlaps) == 0, Overlaps: overlaps,
		Report: filepath.Join(s.WS.ChangeDir(id), filepath.FromSlash(name))}
	if out.Overlaps == nil {
		out.Overlaps = []preflight.Overlap{}
	}
	if card != "" {
		out.Card = &card
	}
	return out, nil
}

// activeWork collects others' work on the repo: open PRs from gh, and from the fetched cache
// the branches pushed in the last 14 days and the commits to base in the last 7.
func (r *runCtx) activeWork(tree worktreeMade) (preflight.Work, error) {
	var w preflight.Work
	out, err := r.tool("gh", "pr", "list", "--repo", ghRepo(tree.Remote), "--state", "open", "--limit", "200",
		"--json", "number,author,headRefName,files")
	if err != nil {
		return w, err
	}
	if w.PRs, err = preflight.ParsePRs(out); err != nil {
		return w, err
	}
	cache, err := r.cachePath(tree.Remote)
	if err != nil {
		return w, err
	}
	now := r.env.Clock.Now()
	refs, err := r.gitOut(cache, "for-each-ref", "--format=%(refname:strip=2)%09%(committerdate:iso-strict)%09%(authoremail)", "refs/heads")
	if err != nil {
		return w, err
	}
	w.Branches = preflight.ParseBranches(refs, now, []string{tree.Base, tree.Branch})
	for i := range w.Branches {
		files, err := r.gitOut(cache, "diff", "--name-only", "refs/heads/"+tree.Base+"...refs/heads/"+w.Branches[i].Name)
		if err != nil {
			return w, err
		}
		w.Branches[i].Files = strings.Fields(files)
	}
	log, err := r.gitOut(cache, "log", "--since="+now.Add(-preflight.CommitWindow).UTC().Format("2006-01-02T15:04:05Z"),
		"--format=%x00%H%x09%ae%x09%cI", "--name-only", "refs/heads/"+tree.Base)
	if err != nil {
		return w, err
	}
	w.Commits = preflight.ParseCommits(log)
	return w, nil
}

// preflightCard raises the A4 card when someone else's open work overlaps, unless this repo's
// preflight card exists already (answered or not); it returns the card's ID, or "".
func (r *runCtx) preflightCard(id, repo string, overlaps []preflight.Overlap) (string, error) {
	if preflight.Blocking(overlaps) == 0 {
		return "", nil
	}
	var card string
	err := r.editDecisions(id, func(f *decisions.File) error {
		for _, e := range f.Decisions {
			if e.Card != nil && e.Card.Question == preflight.Question(repo) {
				card = e.Card.ID
				return nil
			}
		}
		c, err := f.Add(preflight.CardDraft(repo, overlaps))
		if err == nil {
			card = c.ID
		}
		return err
	})
	return card, err
}

// buildingGuard: building starts only after a git preflight of every repo the approved plan
// writes in, with no unanswered card blocking the build (doc 04 §Preflight; FR-GIT-003).
func (r *runCtx) buildingGuard(s *state.Store, id string) error {
	c, err := s.Get(id)
	if err != nil {
		return err
	}
	p, ok := s.ApprovedPlan(c)
	if !ok {
		return guardError(id, state.Building, []string{"the plan is not approved"})
	}
	var problems []string
	var seen []string
	for _, t := range p.Tasks {
		if slices.Contains(seen, t.Repo) {
			continue
		}
		seen = append(seen, t.Repo)
		md, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(id), "preflight", t.Repo+".md"))
		if err != nil {
			problems = append(problems, "no git preflight for "+t.Repo+" (athel git preflight "+id+" "+t.Repo+")")
			continue
		}
		if err := preflight.Check(r.registry(), id, t.Repo, md); err != nil {
			problems = append(problems, err.Error())
		}
	}
	dec, err := r.readDecisions(id)
	if err != nil {
		return err
	}
	for _, card := range dec.Open("") {
		if slices.Contains(card.Blocks, "build") {
			problems = append(problems, card.ID+" is unanswered and blocks the build step")
		}
	}
	return guardError(id, state.Building, problems)
}

// ghRepo is gh's org/repo for a host/org/repo remote.
func ghRepo(remote string) string {
	_, rest, _ := strings.Cut(remote, "/")
	return rest
}

// cachePath is the cache clone of remote, which ensureWorktree has made and fetched.
func (r *runCtx) cachePath(remote string) (string, error) {
	ws, err := r.workspaceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ws.CacheDir(), strings.ReplaceAll(remote, "/", "__")+".git"), nil
}

// gitOut runs a read-only git command with the developer's credentials and returns its output.
func (r *runCtx) gitOut(dir string, args ...string) (string, error) {
	code, out, err := r.networkGit(dir, args...)
	if err != nil || code != 0 {
		detail := strings.TrimSpace(out)
		if err != nil {
			detail = err.Error()
		}
		return "", aerr.New(aerr.ToolGit, "git "+args[0]+": "+detail).WithNextAction("check the cache clone with athel repos cache, then retry")
	}
	return out, nil
}

// tool runs a read-only command-line tool found on the session's PATH, with the developer's
// own login (gh keeps it under HOME), and returns its standard output.
func (r *runCtx) tool(name string, args ...string) ([]byte, error) {
	path := ""
	for _, dir := range filepath.SplitList(r.env.Getenv("PATH")) {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			path = filepath.Join(dir, name)
			break
		}
	}
	if path == "" {
		return nil, aerr.New(aerr.ToolUnreachable, name+" is not on PATH").WithNextAction("install " + name + " and log in, then retry")
	}
	cmd := exec.Command(path, args...) //nolint:gosec // a read-only tool command with fixed subcommands; values are arguments, never shell
	cmd.Env = gitro.NetworkEnv(r.env.Getenv)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		detail := err.Error()
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(stderr.String())
		}
		return nil, aerr.New(aerr.ToolUnreachable, name+" "+args[0]+" "+args[1]+": "+detail).
			WithNextAction("check `" + name + " auth status` and access to the repo, then retry")
	}
	return out.Bytes(), nil
}
