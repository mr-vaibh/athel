package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/changeset"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// The merge-order rehearsal (ADR 036, M5-plan A1): before any PR of a set is marked ready, Athel
// rebuilds each state the system passes through as the PRs merge in the stated order, in
// throwaway clones it never pushes, and runs the build and unit gates of the repos just merged
// and the workspace's system check. "Every intermediate state works" is then shown, not hoped.

const verifySchema = "athel/changeset-verify@1"

type verifyCheck struct {
	Repo   string   `yaml:"repo" json:"repo"`
	Gate   string   `yaml:"gate" json:"gate"`
	Status string   `yaml:"status" json:"status"`
	Tail   []string `yaml:"tail" json:"tail"`
}

type verifySystem struct {
	Status string   `yaml:"status" json:"status"`
	Tail   []string `yaml:"tail" json:"tail"`
}

type verifyStep struct {
	Merged []string      `yaml:"merged" json:"merged"`
	Checks []verifyCheck `yaml:"checks" json:"checks"`
	System verifySystem  `yaml:"system" json:"system"`
}

type verifyReport struct {
	Schema   string            `yaml:"schema" json:"schema"`
	Change   string            `yaml:"change" json:"change"`
	At       string            `yaml:"at" json:"at"`
	Strategy string            `yaml:"strategy" json:"strategy"`
	Result   string            `yaml:"result" json:"result"`
	Heads    map[string]string `yaml:"heads" json:"heads"`
	Steps    []verifyStep      `yaml:"steps" json:"steps"`
}

// rehearsalRepo is one repo the rehearsal needs: its cache clone and base head, and, for a repo
// of the set, the head of Athel's branch there.
type rehearsalRepo struct {
	cache, base, head string
}

func cmdSetVerify(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("set verify"), args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	if c.ChangeSet == nil {
		return nil, aerr.New(aerr.StateInvalid, c.ID+" has no change set; it is derived when the plan is approved")
	}
	system, err := r.systemCheck()
	if err != nil {
		return nil, err
	}
	repos, err := r.rehearsalRepos(s, c, system)
	if err != nil {
		return nil, err
	}
	rep := &verifyReport{Schema: verifySchema, Change: c.ID, At: clock.Format(r.env.Clock.Now()), Strategy: c.ChangeSet.Strategy,
		Result: "pass", Heads: map[string]string{}, Steps: []verifyStep{}}
	for name, rr := range repos {
		rep.Heads[name+"@base"] = rr.base
		if rr.head != "" {
			rep.Heads[name+"@head"] = rr.head
		}
	}
	if err := r.rehearse(s, c, repos, system, rep); err != nil {
		return nil, err
	}
	return rep, r.saveVerify(s, c, session, rep)
}

// systemCheck is the workspace's system check command, nil when none is configured.
func (r *runCtx) systemCheck() ([]string, error) {
	ws, err := r.workspaceDir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(ws.ConfigDir(), "system.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read .athel/config/system.yaml", err)
	}
	if err := r.registry().ValidateYAML("athel/system-check@1", b); err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, ".athel/config/system.yaml", err)
	}
	var cfg struct {
		Command []string `yaml:"command"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, ".athel/config/system.yaml", err)
	}
	return cfg.Command, nil
}

// placeholders are the {repo} words of the system check.
func placeholders(argv []string) []string {
	var out []string
	for _, a := range argv {
		if strings.HasPrefix(a, "{") && strings.HasSuffix(a, "}") && len(a) > 2 && !slices.Contains(out, a[1:len(a)-1]) {
			out = append(out, a[1:len(a)-1])
		}
	}
	return out
}

// rehearsalRepos are the set's repos, each pushed, and the other repos the system check names.
func (r *runCtx) rehearsalRepos(s *state.Store, c *state.Change, system []string) (map[string]rehearsalRepo, error) {
	pol, err := r.policySet()
	if err != nil {
		return nil, err
	}
	out := map[string]rehearsalRepo{}
	for _, e := range c.ChangeSet.Built() {
		w, ok := repoWork(c, e.Repo)
		if !ok || !w.Pushed {
			return nil, aerr.New(aerr.StateInvalid, e.Repo+" of "+c.ID+" is not pushed yet; the rehearsal merges what reviewers see").
				WithNextAction("athel git push " + c.ID + " " + e.Repo)
		}
		head, err := r.gitOut(filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree)), "rev-parse", "HEAD")
		if err != nil {
			return nil, err
		}
		rr, err := r.baseOf(w.Remote, w.Base)
		if err != nil {
			return nil, err
		}
		rr.head = strings.TrimSpace(head)
		out[e.Repo] = rr
	}
	for _, name := range placeholders(system) {
		if _, ok := out[name]; ok {
			continue
		}
		rr, err := r.baseOf(r.resolveRepo(name), pol.Git.BaseBranch)
		if err != nil {
			return nil, err
		}
		out[name] = rr
	}
	return out, nil
}

func (r *runCtx) baseOf(remote, base string) (rehearsalRepo, error) {
	cache, err := r.cachePath(remote)
	if err != nil {
		return rehearsalRepo{}, err
	}
	head, err := r.gitOut(cache, "rev-parse", "--verify", "refs/heads/"+base)
	if err != nil {
		return rehearsalRepo{}, aerr.New(aerr.StateNotFound, "no cache clone of "+remote+" with branch "+base).
			WithNextAction("athel repos cache " + remote)
	}
	return rehearsalRepo{cache: cache, base: strings.TrimSpace(head)}, nil
}

// rehearse builds each state in throwaway clones under the cache folder and checks it.
func (r *runCtx) rehearse(s *state.Store, c *state.Change, repos map[string]rehearsalRepo, system []string, rep *verifyReport) error {
	root := filepath.Join(s.WS.CacheDir(), "rehearsal", c.ID)
	if err := os.RemoveAll(root); err != nil {
		return aerr.Wrap(aerr.StateIO, "clear the rehearsal folder", err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	var before []string
	for n, merged := range c.ChangeSet.Steps() {
		dir := filepath.Join(root, itoa(n+1))
		dirs, err := r.buildState(dir, repos, merged)
		if err != nil {
			return err
		}
		step := verifyStep{Merged: merged, Checks: []verifyCheck{}}
		for _, name := range merged {
			if !slices.Contains(before, name) {
				step.Checks = append(step.Checks, r.stateGates(s, c, name, dirs[name])...)
			}
		}
		step.System = r.runSystem(dir, dirs, system)
		if step.System.Status == "fail" || step.System.Status == "unavailable" ||
			slices.ContainsFunc(step.Checks, func(k verifyCheck) bool { return k.Status != "pass" }) {
			rep.Result = "fail"
		}
		rep.Steps = append(rep.Steps, step)
		before = merged
	}
	return nil
}

// buildState clones every repo at its base and merges the PRs of the merged repos, as a human
// merging them in order would.
func (r *runCtx) buildState(dir string, repos map[string]rehearsalRepo, merged []string) (map[string]string, error) {
	dirs := map[string]string{}
	for name, rr := range repos {
		d := filepath.Join(dir, name)
		if _, err := r.gitOut(filepath.Dir(rr.cache), "clone", "-q", "--shared", "--no-checkout", rr.cache, d); err != nil {
			return nil, err
		}
		if _, err := r.gitOut(d, "checkout", "-q", "--detach", rr.base); err != nil {
			return nil, err
		}
		if slices.Contains(merged, name) {
			if _, err := r.gitOut(d, "-c", "user.name=Athel rehearsal", "-c", "user.email=athel@localhost", "-c", "commit.gpgsign=false",
				"merge", "-q", "--no-ff", "--no-edit", rr.head); err != nil {
				return nil, aerr.Wrap(aerr.ConflictMerge, name+"'s PR does not merge cleanly on its base", err).
					WithNextAction("athel git push merges the base into Athel's branch; then rehearse again")
			}
		}
		dirs[name] = d
	}
	return dirs, nil
}

// stateGates runs a merged repo's build and unit gates in its rehearsal folder.
func (r *runCtx) stateGates(s *state.Store, c *state.Change, name, dir string) []verifyCheck {
	g := gateTask{s: s, c: c, task: plan.Task{Repo: name}, dir: dir}
	files, _ := os.ReadDir(dir)
	var top []string
	for _, f := range files {
		top = append(top, f.Name())
	}
	g.lang = gates.Detect(top)
	list, err := r.gateList(g)
	if err != nil {
		return []verifyCheck{{Repo: name, Gate: "gates.yaml", Status: "unavailable", Tail: []string{err.Error()}}}
	}
	var out []verifyCheck
	for _, gate := range list {
		if gate.ID != "build" && gate.ID != "unit" {
			continue
		}
		out = append(out, r.stateGate(name, dir, gate))
	}
	return out
}

func (r *runCtx) stateGate(name, dir string, gate gates.Gate) verifyCheck {
	k := verifyCheck{Repo: name, Gate: gate.ID, Status: "pass", Tail: []string{}}
	if !gate.Available() || gate.Builtin {
		k.Status, k.Tail = "unavailable", []string{"no " + gate.ID + " command for " + name}
		return k
	}
	if why := r.gateRefusal(dir, gate.Command); why != "" {
		k.Status, k.Tail = "unavailable", []string{"Athel's policy does not let the CLI run it: " + why}
		return k
	}
	exit, out, _, err := r.runGateCommand(dir, gate.Command)
	switch {
	case err != nil:
		k.Status, k.Tail = "unavailable", []string{err.Error()}
	case exit != 0:
		k.Status, k.Tail = "fail", tail(out)
	}
	return k
}

// runSystem runs the system check in a state, each {repo} word replaced by its folder.
func (r *runCtx) runSystem(dir string, dirs map[string]string, system []string) verifySystem {
	if len(system) == 0 {
		return verifySystem{Status: "not_configured", Tail: []string{"no .athel/config/system.yaml: only each repo's own gates ran"}}
	}
	argv := make([]string, len(system))
	for i, a := range system {
		argv[i] = a
		if strings.HasPrefix(a, "{") && strings.HasSuffix(a, "}") {
			argv[i] = dirs[a[1:len(a)-1]]
		}
	}
	if why := r.gateRefusal(dir, argv); why != "" {
		return verifySystem{Status: "unavailable", Tail: []string{"Athel's policy does not let the CLI run it: " + why}}
	}
	exit, out, _, err := r.runGateCommand(dir, argv)
	switch {
	case err != nil:
		return verifySystem{Status: "unavailable", Tail: []string{err.Error()}}
	case exit != 0:
		return verifySystem{Status: "fail", Tail: tail(out)}
	}
	return verifySystem{Status: "pass", Tail: tail(out)}
}

// saveVerify writes the evidence and records the result and heads in the change set.
func (r *runCtx) saveVerify(s *state.Store, c *state.Change, session string, rep *verifyReport) error {
	b, err := yaml.Marshal(rep)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "encode the rehearsal", err)
	}
	if err := r.registry().ValidateYAML(verifySchema, b); err != nil {
		return err
	}
	if _, err := s.SaveChangeFile(c.ID, session, "changeset/verify.yaml", b); err != nil {
		return err
	}
	_, err = s.SetVerify(c.ID, session, &changeset.Verify{At: rep.At, Result: rep.Result, Heads: rep.Heads})
	return err
}

// rehearsalProblem says why a set's PRs may not be marked ready yet: no rehearsal, a failed one,
// or one run on other heads than now (a push or a base that moved; M5-plan G7, G14). "" means
// the last rehearsal passed on exactly what reviewers see. A one-repo change needs none.
func (r *runCtx) rehearsalProblem(s *state.Store, c *state.Change) (string, error) {
	if c.ChangeSet == nil || len(c.ChangeSet.Built()) < 2 {
		return "", nil
	}
	v := c.ChangeSet.Verify
	switch {
	case v == nil:
		return "the merge order of " + c.ID + " has not been rehearsed", nil
	case v.Result != "pass":
		return "the last merge-order rehearsal of " + c.ID + " failed; see changeset/verify.yaml", nil
	}
	system, err := r.systemCheck()
	if err != nil {
		return "", err
	}
	repos, err := r.rehearsalRepos(s, c, system)
	if err != nil {
		return "", err
	}
	for name, rr := range repos {
		if v.Heads[name+"@base"] != rr.base || rr.head != "" && v.Heads[name+"@head"] != rr.head {
			return "the merge-order rehearsal of " + c.ID + " is stale: " + name + " moved since it ran", nil
		}
	}
	return "", nil
}
