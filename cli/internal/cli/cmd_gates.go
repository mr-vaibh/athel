package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/scrub"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// Doc 06's gates, run by the CLI in a task's worktree (FR-BUILD-009; ADR 023): the baseline on
// the untouched worktree, then each run judged against it, with the evidence in
// gates/<task>.yaml and every output under artifacts/.

const gateResultSchema = "athel/gate-result@1"

// gateTimeout bounds one gate command.
const gateTimeout = 10 * time.Minute

// flakyRuns is doc 06's "re-run up to 3 times".
const flakyRuns = 3

type gateOutcome struct {
	Gate       string   `yaml:"gate" json:"gate"`
	Status     string   `yaml:"status" json:"status"` // pass | fail | unavailable
	Summary    string   `yaml:"summary" json:"summary"`
	Details    []string `yaml:"details" json:"details"`
	Command    []string `yaml:"command" json:"command"`
	Exit       int      `yaml:"exit" json:"exit"`
	DurationMS int64    `yaml:"duration_ms" json:"duration_ms"`
	Output     *string  `yaml:"output" json:"output"`
	Flaky      []string `yaml:"flaky" json:"flaky"`
}

type gateRun struct {
	N       int           `yaml:"n" json:"n"`
	Stage   string        `yaml:"stage" json:"stage"`
	Head    string        `yaml:"head" json:"head"`
	At      string        `yaml:"at" json:"at"`
	Green   bool          `yaml:"green" json:"green"`
	Results []gateOutcome `yaml:"results" json:"results"`
}

type gateBaseline struct {
	Head  string         `yaml:"head" json:"head"`
	At    string         `yaml:"at" json:"at"`
	Lint  string         `yaml:"lint" json:"lint"`
	Tests gates.TestRun  `yaml:"tests" json:"tests"`
	Exits map[string]int `yaml:"exits" json:"exits"`
	// Coverage is the total statement coverage on base, when the coverage gate ran there.
	Coverage *float64 `yaml:"coverage" json:"coverage"`
}

type gateFile struct {
	Schema   string        `yaml:"schema" json:"schema"`
	Change   string        `yaml:"change" json:"change"`
	Task     string        `yaml:"task" json:"task"`
	Repo     string        `yaml:"repo" json:"repo"`
	Baseline *gateBaseline `yaml:"baseline" json:"baseline"`
	Runs     []gateRun     `yaml:"runs" json:"runs"`
}

// gateTask is a task of a building change, with its worktree.
type gateTask struct {
	s       *state.Store
	session string
	c       *state.Change
	task    plan.Task
	dir     string
	lang    string
	base    string // the commit the change's branch was cut from
}

func (r *runCtx) gateTask(args []string, name string) (gateTask, error) {
	pos, err := parse(r.newFlags(name), args, 2)
	if err != nil {
		return gateTask{}, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return gateTask{}, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return gateTask{}, err
	}
	if c.State != state.Building {
		return gateTask{}, aerr.New(aerr.StateInvalid, c.ID+" is "+string(c.State)+"; gates run while building")
	}
	p, _ := s.ApprovedPlan(c) // building needs an approved plan (buildingGuard)
	g := gateTask{s: s, session: session, c: c}
	found := false
	for _, t := range p.Tasks {
		if t.ID == pos[1] {
			g.task, found = t, true
		}
	}
	if !found {
		return gateTask{}, aerr.New(aerr.StateNotFound, c.ID+"'s plan has no task "+pos[1])
	}
	w, ok := repoWork(c, g.task.Repo)
	if !ok {
		return gateTask{}, aerr.New(aerr.StateNotFound, c.ID+" has no worktree in "+g.task.Repo).WithNextAction("athel git preflight " + c.ID + " " + g.task.Repo)
	}
	g.dir, g.base = filepath.Join(s.WS.Root, filepath.FromSlash(w.Worktree)), w.BaseHead
	entries, err := os.ReadDir(g.dir)
	if err != nil {
		return gateTask{}, aerr.Wrap(aerr.StateIO, "read "+g.dir, err)
	}
	var files []string
	for _, e := range entries {
		files = append(files, e.Name())
	}
	g.lang = gates.Detect(files)
	return g, nil
}

// gateList is the task repo's gates: the defaults, with the repo's gates.yaml overrides.
func (r *runCtx) gateList(g gateTask) ([]gates.Gate, error) {
	b, err := os.ReadFile(filepath.Join(g.s.WS.Root, ".athel", "config", "gates.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return g.defaults(), nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read .athel/config/gates.yaml", err)
	}
	if err := r.registry().ValidateYAML("athel/gates@1", b); err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, ".athel/config/gates.yaml", err)
	}
	var cfg struct {
		Repos map[string][]gates.Gate `yaml:"repos"`
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, ".athel/config/gates.yaml", err)
	}
	list, err := gates.Override(g.defaults(), cfg.Repos[g.task.Repo])
	if err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, ".athel/config/gates.yaml", err)
	}
	return list, nil
}

func (g gateTask) defaults() []gates.Gate {
	scripts := map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(g.dir, "package.json")); err == nil {
		var pkg struct{ Scripts map[string]string }
		if json.Unmarshal(b, &pkg) == nil {
			for name := range pkg.Scripts {
				scripts[name] = true
			}
		}
	}
	return gates.Defaults(g.lang, scripts)
}

func (r *runCtx) loadGates(g gateTask) (*gateFile, error) {
	f := &gateFile{Schema: gateResultSchema, Change: g.c.ID, Task: g.task.ID, Repo: g.task.Repo, Runs: []gateRun{}}
	b, err := os.ReadFile(filepath.Join(g.s.WS.ChangeDir(g.c.ID), "gates", g.task.ID+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read the gate results", err)
	}
	if err := r.registry().ValidateYAML(gateResultSchema, b); err != nil {
		return nil, err
	}
	return f, yaml.Unmarshal(b, f)
}

func (r *runCtx) saveGates(g gateTask, f *gateFile) error {
	b, err := yaml.Marshal(f)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "encode the gate results", err)
	}
	if err := r.registry().ValidateYAML(gateResultSchema, b); err != nil {
		return err
	}
	_, err = g.s.SaveChangeFile(g.c.ID, g.session, "gates/"+g.task.ID+".yaml", b)
	return err
}

// cmdGateBaseline runs the task's gates on the untouched worktree (doc 06 build loop step 2),
// once: later failures are told apart from ones already on base.
func cmdGateBaseline(r *runCtx, args []string) (any, error) {
	g, err := r.gateTask(args, "gate baseline")
	if err != nil {
		return nil, err
	}
	f, err := r.loadGates(g)
	if err != nil || f.Baseline != nil {
		return f, err
	}
	if dirty, err := r.gitOut(g.dir, "status", "--porcelain"); err != nil || strings.TrimSpace(dirty) != "" {
		if err == nil {
			err = aerr.New(aerr.StateInvalid, "the worktree has uncommitted changes; the baseline is taken before the task's first edit")
		}
		return nil, err
	}
	head, err := r.gitOut(g.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	list, err := r.gateList(g)
	if err != nil {
		return nil, err
	}
	b := &gateBaseline{Head: strings.TrimSpace(head), At: clock.Format(r.env.Clock.Now()), Exits: map[string]int{}, Tests: gates.TestRun{Tests: map[string]string{}, BrokenPackages: []string{}}}
	for _, gate := range list {
		if len(gate.Command) == 0 {
			continue
		}
		argv, profile, done := withProfile(gate.Command)
		exit, out, err := 0, "", errors.New(r.gateRefusal(g.dir, argv))
		if err.Error() == "" {
			exit, out, _, err = r.runGateCommand(g.dir, argv)
		}
		if err == nil {
			b.record(gate, g, exit, out, profile)
		}
		done()
	}
	f.Baseline = b
	return f, r.saveGates(g, f)
}

// cmdGateRun runs a stage's gates and judges them against the baseline.
func cmdGateRun(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("gate run")
	stage := fs.String("stage", gates.Cheap, "cheap | full")
	pos, err := parse(fs, args, 2)
	if err != nil {
		return nil, err
	}
	if *stage != gates.Cheap && *stage != gates.Full {
		return nil, usagef("--stage is cheap or full")
	}
	g, err := r.gateTask(pos, "gate run")
	if err != nil {
		return nil, err
	}
	f, err := r.loadGates(g)
	if err != nil {
		return nil, err
	}
	if f.Baseline == nil {
		return nil, aerr.New(aerr.StateInvalid, "task "+g.task.ID+" has no baseline yet").WithNextAction("athel gate baseline " + g.c.ID + " " + g.task.ID)
	}
	head, err := r.gitOut(g.dir, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	list, err := r.gateList(g)
	if err != nil {
		return nil, err
	}
	run := gateRun{N: len(f.Runs) + 1, Stage: *stage, Head: strings.TrimSpace(head), At: clock.Format(r.env.Clock.Now()), Green: true, Results: []gateOutcome{}}
	for _, gate := range list {
		if gate.Stage != *stage && *stage == gates.Cheap {
			continue
		}
		o := r.evaluateGate(g, f.Baseline, gate, run.N)
		run.Green = run.Green && (o.Status == gates.Pass || o.Status == gates.Skipped)
		run.Results = append(run.Results, o)
	}
	f.Runs = append(f.Runs, run)
	if err := r.saveGates(g, f); err != nil {
		return nil, err
	}
	return run, redError(g, run)
}

// redError names every gate that is not green, or is nil.
func redError(g gateTask, run gateRun) error {
	var red []string
	for _, o := range run.Results {
		if o.Status != gates.Pass && o.Status != gates.Skipped {
			red = append(red, o.Gate+" "+o.Status+": "+o.Summary)
		}
	}
	if len(red) == 0 {
		return nil
	}
	return aerr.New(aerr.GateRed, "task "+g.task.ID+" gates are red: "+strings.Join(red, "; ")).
		WithNextAction("fix what the gates report and run them again; athel gate show " + g.c.ID + " " + g.task.ID + " has the details")
}

// cmdGateShow prints a task's gate evidence.
func cmdGateShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("gate show"), args, 2)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(s.WS.ChangeDir(pos[0]), "gates", pos[1]+".yaml"))
	if err != nil {
		return nil, aerr.Wrap(aerr.StateNotFound, pos[0]+" has no gate results for "+pos[1], err)
	}
	var f gateFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "gates/"+pos[1]+".yaml", err)
	}
	return f, nil
}

// evaluateGate runs one gate and judges it.
func (r *runCtx) evaluateGate(g gateTask, base *gateBaseline, gate gates.Gate, n int) gateOutcome {
	o := gateOutcome{Gate: gate.ID, Command: append([]string{}, gate.Command...), Details: []string{}, Flaky: []string{}}
	if o.Command == nil {
		o.Command = []string{}
	}
	switch {
	case !gate.Available():
		o.Status, o.Summary = "unavailable", "no "+gate.ID+" command for this repo (doc 06: the gate blocks unless a human waives it)"
		return o
	case gate.Builtin:
		o.Status = gates.Pass
		r.judgeBuiltin(g, base, gate, &o)
		return o
	}
	argv, profile, done := withProfile(gate.Command)
	defer done()
	if why := r.gateRefusal(g.dir, argv); why != "" {
		o.Status, o.Summary = "unavailable", "Athel's policy does not let the CLI run it: "+why
		return o
	}
	exit, out, dur, err := r.runGateCommand(g.dir, argv)
	o.Exit, o.DurationMS = exit, dur.Milliseconds()
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return o
	}
	name := fmt.Sprintf("artifacts/gates/%s/%d-%s.log", g.task.ID, n, gate.ID)
	if _, err := g.s.SaveChangeFile(g.c.ID, g.session, name, []byte(out)); err == nil {
		o.Output = &name
	}
	o.Status = gates.Pass
	switch {
	case gate.Pass == gates.NoOutput && (exit != 0 || strings.TrimSpace(out) != ""):
		o.Status, o.Summary, o.Details = gates.Fail, "the formatter wants to change files", tail(out)
	case gate.Pass == gates.NewFindings:
		r.judgeLint(g, base, out, &o)
	case gate.Pass == gates.Tests && g.lang == gates.Go:
		r.judgeTests(g, base, out, &o)
	case gate.Pass == gates.Coverage:
		r.judgeCoverage(g, base, exit, out, profile, &o)
	case exit != 0:
		o.Status, o.Summary, o.Details = gates.Fail, gate.ID+" exited "+strconv.Itoa(exit), tail(out)
	}
	return o
}

func (r *runCtx) judgeLint(g gateTask, base *gateBaseline, out string, o *gateOutcome) {
	added, err := r.gitOut(g.dir, "diff", "-U0", "--no-color", "--no-ext-diff", base.Head)
	var plus []string
	for _, l := range strings.Split(added, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			plus = append(plus, l[1:])
		}
	}
	found := append(gates.NewLines(base.Lint, out), gates.DisableComments(strings.Join(plus, "\n"))...)
	switch {
	case err != nil:
		o.Status, o.Summary = "unavailable", err.Error()
	case len(found) > 0:
		o.Status, o.Summary, o.Details = gates.Fail, strconv.Itoa(len(found))+" new lint findings or disable comments", found
	}
}

func (r *runCtx) judgeTests(g gateTask, base *gateBaseline, out string, o *gateOutcome) {
	v := gates.CompareTests(base.Tests, gates.ParseGoTest(out))
	v.Regressed = r.dropBaseFlaky(g, base, v.Regressed, o)
	if v.Green() {
		return
	}
	o.Status, o.Summary = gates.Fail, "tests fail, are skipped or were deleted compared with the baseline"
	for label, ids := range map[string][]string{"regressed": v.Regressed, "new failing": v.NewFailing, "deleted": v.Deleted, "skipped": v.Skipped, "does not build": v.Broken} {
		for _, id := range ids {
			o.Details = append(o.Details, label+": "+id)
		}
	}
	sort.Strings(o.Details)
}

// dropBaseFlaky re-runs each regressed test up to 3 times. A test that passes again is flaky; it
// stops counting against the task only if it also flakes on the baseline (doc 06 edge case).
func (r *runCtx) dropBaseFlaky(g gateTask, base *gateBaseline, regressed []string, o *gateOutcome) []string {
	var keep []string
	for _, id := range regressed {
		if !r.passesOnce(g.dir, id) {
			keep = append(keep, id)
			continue
		}
		if r.flakesOnBase(g, base.Head, id) {
			o.Flaky = append(o.Flaky, id+" (flaky on base too: not this task's failure)")
			continue
		}
		o.Flaky = append(o.Flaky, id+" (flaky only after this change)")
		keep = append(keep, id)
	}
	return keep
}

// passesOnce reports whether a test passes in one of up to 3 runs.
func (r *runCtx) passesOnce(dir, id string) bool {
	pkg, name := gates.SplitTestID(id)
	argv := []string{"go", "test", "-json", "-count=1", "-run", gates.RunPattern(name), pkg}
	for range flakyRuns {
		if _, out, _, err := r.runGateCommand(dir, argv); err == nil && gates.ParseGoTest(out).Tests[id] == gates.Pass {
			return true
		}
	}
	return false
}

// flakesOnBase runs a test 3 times on a throwaway checkout of the baseline head.
func (r *runCtx) flakesOnBase(g gateTask, head, id string) bool {
	tmp, err := os.MkdirTemp("", "athel-base-")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(tmp) }() // a throwaway checkout
	dir := filepath.Join(tmp, "base")
	if _, err := r.gitOut(g.dir, "worktree", "add", "--quiet", "--detach", dir, head); err != nil {
		return false
	}
	defer func() { _, _ = r.gitOut(g.dir, "worktree", "remove", "--force", dir) }() // best effort; the folder is removed anyway
	pkg, name := gates.SplitTestID(id)
	argv := []string{"go", "test", "-json", "-count=1", "-run", gates.RunPattern(name), pkg}
	for range flakyRuns {
		if _, out, _, err := r.runGateCommand(dir, argv); err == nil && gates.ParseGoTest(out).Tests[id] == gates.Fail {
			return true
		}
	}
	return false
}

// gateRefusal is why the CLI may not run argv in dir (ADR 023), or "".
func (r *runCtx) gateRefusal(dir string, argv []string) string {
	pol, _ := r.policySet() // a broken policy leaves nil: the classifier then knows no tools.yaml
	return gateRefusal(pol, dir, argv)
}

func gateRefusal(pol *policy.Set, dir string, argv []string) string {
	words := make([]string, len(argv))
	for i, a := range argv {
		words[i] = shellQuote(a)
	}
	in, _ := json.Marshal(map[string]string{"command": strings.Join(words, " ")}) // strings always encode
	res := classify.Classify(classify.Call{Tool: "Bash", Input: in, Cwd: dir}, pol, classify.Options{ReadFile: readFile})
	if res.Unparseable {
		return "the command cannot be read"
	}
	for _, a := range res.Actions {
		switch {
		case a.Forbidden != "":
			return a.Forbidden
		case a.Uncertain != "":
			return a.Text + ": Athel does not know what it does (" + a.Uncertain + ")"
		case a.Tier != classify.R0 && a.Tier != classify.W0:
			return a.Text + " is tier " + string(a.Tier) + "; gates run only local commands"
		}
		for _, p := range a.Paths {
			if rel, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(rel, "..") {
				return a.Text + " writes " + p + ", outside the worktree"
			}
		}
	}
	return ""
}

// gateEnv is the scrubbed environment gate commands run with (ADR 023).
func (r *runCtx) gateEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "GOPATH", "GOCACHE", "GOMODCACHE", "GOFLAGS", "GOTOOLCHAIN", "GOPROXY", "NODE_PATH", "npm_config_cache"} {
		if v := r.env.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "CI=true")
}

// runGateCommand runs argv in dir: its exit code, its output with secrets scrubbed, and how
// long it took. err means the command could not run at all (tool missing, timeout).
func (r *runCtx) runGateCommand(dir string, argv []string) (int, string, time.Duration, error) {
	path := lookPath(r.env.Getenv("PATH"), argv[0])
	if path == "" {
		return 0, "", 0, errors.New(argv[0] + " is not installed (not on PATH)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), gateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, argv[1:]...) //nolint:gosec // argv passed ADR 023's check; never a shell
	cmd.Dir, cmd.Env = dir, r.gateEnv()
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)
	text, _ := scrub.New().String(out.String())
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return exit.ExitCode(), text, took, nil
	case ctx.Err() != nil:
		return 0, text, took, errors.New(argv[0] + " did not finish within " + gateTimeout.String())
	case err != nil:
		return 0, text, took, err
	}
	return 0, text, took, nil
}

// lookPath finds name on a PATH value.
func lookPath(pathEnv, name string) string {
	for _, dir := range filepath.SplitList(pathEnv) {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() { //nolint:gosec // the developer's own PATH, searched for a tool name Athel fixed
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// tail is the last lines of a command's output, for the evidence.
func tail(out string) []string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return lines
}
