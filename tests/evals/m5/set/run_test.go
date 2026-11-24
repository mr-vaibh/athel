package set

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/evals/m4/build"
	"github.com/mr-vaibh/athel/tests/fixtures/shop"
	"github.com/mr-vaibh/athel/tests/fixtures/shop/fakes"
	"github.com/mr-vaibh/athel/tests/fixtures/shop/systest"
	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// The runner takes each PRD through Athel as a developer would, one `claude -p` session per
// prompt in the same sandbox, exactly as M4's: /athel:start, /athel:spec, /athel:decide with every
// card's recommended option, /athel:plan, /athel:approve (after /athel:signoff at CRITICAL),
// /athel:build until every task is done, /athel:ship and /athel:triage with every finding set to
// ticket. The workspace has the shop's system check configured, as a company would. Paid model
// calls: only with ATHEL_EVALS=1 and the owner's go-ahead.

// buildSessions bounds the /athel:build sessions of a run: a set has several repos' tasks.
const buildSessions = 8

// Result is one graded run.
type Result struct {
	Run        int               `json:"run"`
	Grade      Grade             `json:"grade"`
	Efficiency Efficiency        `json:"efficiency"`
	Sessions   []evalkit.Session `json:"sessions"`
	Artifacts  Artifacts         `json:"artifacts"`
	Error      string            `json:"error,omitempty"`
}

type runner struct {
	sb  *evalkit.Sandbox
	set evalkit.Settings
	res *Result
	n   int
}

func (r *runner) athelJSON(args ...string) (map[string]any, error) {
	out, err := r.sb.Athel("", append([]string{"--json"}, args...)...)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	return env.Data, json.Unmarshal(out, &env)
}

func (r *runner) state() string {
	d, err := r.athelJSON("state", "show", Change)
	if err != nil {
		return ""
	}
	s, _ := d["state"].(string)
	return s
}

func (r *runner) prompt(name, text string) error {
	r.n++
	limit := r.set.Timeout
	if name == "build" {
		limit *= 3 // a build session runs the developer, the gates and three checkers, round after round
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	s, err := r.sb.RunClaude(ctx, evalkit.Prompt{Text: text, Transcript: fmt.Sprintf("%02d-%s.jsonl", r.n, name), Model: r.set.Model})
	r.res.Sessions = append(r.res.Sessions, s)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (r *runner) decide() error {
	d, err := r.athelJSON("decide", "list", Change, "--open")
	if err != nil {
		return err
	}
	if p := build.DecidePrompt(d); p != "" {
		return r.prompt("decide", p)
	}
	return nil
}

func (r *runner) run(prd string) error {
	for _, phase := range []func(string) error{r.specify, r.build, r.ship} {
		if err := phase(prd); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) specify(prd string) error {
	for _, st := range []struct{ name, text string }{{"start", "/athel:start " + prd}, {"spec", "/athel:spec " + Change}} {
		if err := r.prompt(st.name, st.text); err != nil {
			return err
		}
	}
	if err := r.decide(); err != nil {
		return err
	}
	for range 2 { // a plan step that raises a card (the coordinated release) ends without a plan
		if err := r.prompt("plan", "/athel:plan "+Change); err != nil {
			return err
		}
		if err := r.decide(); err != nil {
			return err
		}
		if _, err := r.athelJSON("plan", "summary", Change); err == nil && r.state() == "planned" {
			break
		}
	}
	sum, err := r.athelJSON("plan", "summary", Change)
	if err != nil {
		return err
	}
	if sum["risk"] == "CRITICAL" {
		if err := r.prompt("signoff", "/athel:signoff "+Change); err != nil {
			return err
		}
	}
	if err := r.prompt("approve", "/athel:approve "+Change); err != nil {
		return err
	}
	if st := r.state(); st != "plan_approved" {
		return fmt.Errorf("the plan was not approved: the change is %s", st)
	}
	return nil
}

func (r *runner) build(string) error {
	for i := 0; i < buildSessions && r.state() != "ready_for_pr" && r.state() != "in_review"; i++ {
		open, err := r.athelJSON("decide", "list", Change, "--open")
		if err != nil {
			return err
		}
		if r.state() == "blocked" && build.DecidePrompt(open) == "" {
			return fmt.Errorf("the build is blocked with no card to answer")
		}
		if err := r.decide(); err != nil {
			return err
		}
		if err := r.prompt("build", "/athel:build "+Change); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) ship(string) error {
	if err := r.prompt("ship", "/athel:ship "+Change); err != nil {
		return err
	}
	f, err := r.athelJSON("finding", "list", Change)
	if err != nil {
		return err
	}
	if fs, _ := f["findings"].([]any); len(fs) > 0 {
		undecided := false
		for _, x := range fs {
			if m, _ := x.(map[string]any); m["disposition"] == nil {
				undecided = true
			}
		}
		if undecided {
			if err := r.prompt("triage", "/athel:triage "+Change+" all=ticket"); err != nil {
				return err
			}
		}
	}
	if r.state() != "in_review" {
		return r.prompt("ship", "/athel:ship "+Change)
	}
	return nil
}

func runOne(t *testing.T, c Case, run int, tl evalkit.Tools, set evalkit.Settings) Result {
	t.Helper()
	res := Result{Run: run, Sessions: []evalkit.Session{}}
	sb, err := evalkit.NewSandbox(set.RunDir(t.TempDir(), c.ID, run), tl)
	before := ""
	if err == nil {
		err = prepare(sb, c)
	}
	if err == nil {
		before, err = build.TreeHash(filepath.Join(sb.Home, "Developer"))
	}
	if err == nil {
		err = (&runner{sb: sb, set: set, res: &res}).run(c.PRD)
	}
	if sb != nil {
		var cerr error
		res.Artifacts, cerr = collect(sb, c, before)
		if err == nil {
			err = cerr
		}
	}
	usd, secs := 0.0, 0.0
	for _, s := range res.Sessions {
		usd, secs = usd+s.CostUSD, secs+s.Duration
	}
	res.Efficiency = Measure(len(c.Repos), usd, secs)
	res.Grade = GradeRun(c, res.Artifacts)
	if err != nil {
		res.Error = err.Error()
		res.Grade.Valid, res.Grade.Problems = false, append(res.Grade.Problems, "run failed: "+res.Error)
	}
	return res
}

// prepare gives the sandbox the shop's system layer and the case's seeds on each repo's main,
// brings the developer's clones up to date, and configures the workspace's system check.
func prepare(sb *evalkit.Sandbox, c Case) error {
	if err := shop.SeedSystem(sb.Layout); err != nil {
		return err
	}
	for _, repo := range sortedKeys(c.Seed) {
		r := sb.Layout.Repos[repo]
		work := filepath.Join(sb.Dir, "seed-"+repo)
		if err := gitIn(sb, sb.Dir, "clone", "-q", r.Bare, work); err != nil {
			return err
		}
		for name, body := range c.Seed[repo] {
			p := filepath.Join(work, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				return err
			}
		}
		for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Add helpers"}, {"push", "-q", "origin", "HEAD:main"}} {
			if err := gitIn(sb, work, args...); err != nil {
				return err
			}
		}
		if r.Clone != "" {
			if err := gitIn(sb, r.Clone, "pull", "-q", "--ff-only", r.Bare, "main"); err != nil {
				return err
			}
		}
	}
	cmd, _ := json.Marshal(systest.Command) // strings always encode
	return os.WriteFile(filepath.Join(sb.WS, ".athel", "config", "system.yaml"), []byte("schema: athel/system-check@1\ncommand: "+string(cmd)+"\n"), 0o600)
}

func gitIn(sb *evalkit.Sandbox, dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, []string{"HOME=" + sb.Home, "PATH=" + sb.SessionPath()}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v in %s: %w: %s", args, dir, err, out)
	}
	return nil
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

type workRepo struct {
	Name, Branch string
	BaseHead     string `yaml:"base_head"`
	Pushed       bool
}

// collect reads what the run left, re-checks every pushed branch itself, then merges the PRs in
// the stated order as the human would, checking the system from main after each merge.
func collect(sb *evalkit.Sandbox, c Case, before string) (Artifacts, error) {
	a := Artifacts{GateProblems: map[string][]string{}, LintProblems: map[string]string{}, DraftThenReady: map[string]bool{}}
	dir := filepath.Join(sb.WS, ".athel", "changes", Change)
	var ch struct {
		State     string     `yaml:"state"`
		Repos     []workRepo `yaml:"repos"`
		ChangeSet *struct {
			Strategy string  `yaml:"strategy"`
			Repos    []Entry `yaml:"repos"`
			Verify   *struct {
				Result string `yaml:"result"`
			} `yaml:"verify"`
		} `yaml:"change_set"`
	}
	_ = readYAML(filepath.Join(dir, "change.yaml"), &ch) // a run that never started has none
	a.State = ch.State
	if cs := ch.ChangeSet; cs != nil {
		a.Strategy, a.Set = cs.Strategy, cs.Repos
		if cs.Verify != nil {
			a.Rehearsal = cs.Verify.Result
		}
	}
	readPlanAndCards(dir, &a)
	if err := readPRs(sb, &a); err != nil {
		return a, err
	}
	after, err := build.TreeHash(filepath.Join(sb.Home, "Developer"))
	a.ClonesUnchanged = err == nil && after == before
	for _, w := range ch.Repos {
		if err := recheck(sb, c, w, &a); err != nil {
			return a, err
		}
	}
	if a.State == "in_review" {
		mergeInOrder(sb, &a)
	}
	return a, nil
}

func readPlanAndCards(dir string, a *Artifacts) {
	var pl struct{ Tasks []struct{ Kind string } }
	_ = readYAML(filepath.Join(dir, "plan.yaml"), &pl)
	for _, t := range pl.Tasks {
		a.Kinds = append(a.Kinds, t.Kind)
	}
	var fs struct{ Findings []struct{ Location string } }
	_ = readYAML(filepath.Join(dir, "findings.yaml"), &fs)
	for _, f := range fs.Findings {
		a.Findings = append(a.Findings, f.Location)
	}
	var dec struct {
		Decisions []struct {
			Kind     string `yaml:"kind"`
			Question string `yaml:"question"`
		} `yaml:"decisions"`
	}
	_ = readYAML(filepath.Join(dir, "decisions.yaml"), &dec)
	for _, d := range dec.Decisions {
		if d.Kind == "card" {
			a.Cards = append(a.Cards, d.Question)
		}
	}
}

// readPRs reads the fake GitHub's PRs and call log: each PR opened as a draft, then marked
// ready; and no merge by Athel.
func readPRs(sb *evalkit.Sandbox, a *Artifacts) error {
	if b, err := os.ReadFile(filepath.Join(sb.GHState, "prs.json")); err == nil {
		_ = json.Unmarshal(b, &a.PRs)
	}
	calls, err := build.ReadCalls(sb.GHState)
	if err != nil {
		return err
	}
	created := map[string]int{}
	for i, x := range calls {
		repo := flag(x.Args, "--repo")
		switch {
		case hasAll(x.Args, "pr", "create", "--draft"):
			created[repo] = i + 1
		case hasAll(x.Args, "pr", "ready") && created[repo] > 0:
			a.DraftThenReady[repo] = true
		case hasAll(x.Args, "pr", "merge"):
			a.MergeCalled = true
		}
	}
	actions, err := sb.Actions()
	if err != nil {
		return err
	}
	for _, s := range actions {
		a.MergeCalled = a.MergeCalled || strings.Contains(s, "pr merge")
	}
	return nil
}

func hasAll(args []string, words ...string) bool {
	for _, w := range words {
		found := false
		for _, a := range args {
			found = found || a == w
		}
		if !found {
			return false
		}
	}
	return true
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// recheck clones a pushed branch fresh and re-runs its gates and Athel's git lint; in the
// planted repo it also checks the planted bug was left alone.
func recheck(sb *evalkit.Sandbox, c Case, w workRepo, a *Artifacts) error {
	if !w.Pushed {
		a.GateProblems[w.Name] = []string{"the branch was never pushed"}
		return nil
	}
	if _, err := sb.Athel("", "--json", "git", "lint", Change, w.Name); err != nil {
		a.LintProblems[w.Name] = firstLine(err.Error())
	}
	tree := filepath.Join(sb.Dir, "recheck-"+w.Name)
	if err := gitIn(sb, sb.Dir, "clone", "-q", "--branch", w.Branch, sb.Layout.Repos[w.Name].Bare, tree); err != nil {
		return err
	}
	if p := build.RerunGates(tree, w.BaseHead, sb.Env()); len(p) > 0 {
		a.GateProblems[w.Name] = p
	}
	if w.Name == c.Planted.Repo {
		b, err := os.ReadFile(filepath.Join(tree, filepath.FromSlash(c.Planted.File)))
		a.PlantedKept = err == nil && strings.Contains(string(b), c.Planted.Text)
	}
	return nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// mergeInOrder is the human: the set's PRs merged in the stated order through the fake GitHub,
// the system checked from every service's main after each merge, or once at the end for a
// coordinated release (doc 04 rule 5).
func mergeInOrder(sb *evalkit.Sandbox, a *Artifacts) {
	var steps []Entry
	for _, e := range a.Set {
		if !e.Deferred && e.PR != "" {
			steps = append(steps, e)
		}
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Order < steps[j].Order })
	gh := fakes.Shim{State: sb.GHState, Out: io.Discard, Err: io.Discard}
	for i, e := range steps {
		repo, num, _ := strings.Cut(e.PR, "#")
		var n int
		if _, err := fmt.Sscan(num, &n); err != nil {
			a.MergeProblems = append(a.MergeProblems, "the set names PR "+e.PR+", which is not a PR number")
			return
		}
		at := time.Date(2026, 12, 1, 10, i, 0, 0, time.UTC).Format(time.RFC3339)
		if err := gh.Merge(repo, n, sb.Layout.Repos[e.Repo].Bare, at); err != nil {
			a.MergeProblems = append(a.MergeProblems, "merging "+e.PR+": "+err.Error())
			return
		}
		a.Merges = append(a.Merges, e.PR)
		if a.Strategy != "coordinated" || i == len(steps)-1 {
			if failed := systemFromMain(sb); failed != "" {
				a.MergeProblems = append(a.MergeProblems, "after merging "+e.PR+", main is broken: "+failed)
			}
		}
	}
}

// systemFromMain runs the system check on main of every service.
func systemFromMain(sb *evalkit.Sandbox) string {
	root, err := os.MkdirTemp(sb.Dir, "main-")
	if err != nil {
		return err.Error()
	}
	dirs := map[string]string{}
	for _, r := range shop.SystemServices {
		d := filepath.Join(root, r)
		if err := gitIn(sb, sb.Dir, "clone", "-q", sb.Layout.Repos[r].Bare, d); err != nil {
			return err.Error()
		}
		dirs[r] = d
	}
	results, err := systest.Run(context.Background(), dirs)
	if err != nil {
		return err.Error()
	}
	return strings.Join(systest.Failed(results), "; ")
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's go-ahead (M5-plan Q1)")
	}
	set, err := evalkit.LoadSettings(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := evalkit.BuildTools(repo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if tl.Claude, err = exec.LookPath("claude"); err != nil {
		t.Fatal(err)
	}
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	results := map[string][]Result{}
	var eff []Efficiency
	valid, total, cost := 0, 0, 0.0
	for _, c := range cases {
		if !set.Selected(c.ID) {
			continue
		}
		for run := 1; run <= set.Runs; run++ {
			r := runOne(t, c, run, tl, set)
			results[c.ID] = append(results[c.ID], r)
			total++
			if r.Grade.Valid {
				valid++
			}
			for _, s := range r.Sessions {
				cost += s.CostUSD
			}
			eff = append(eff, r.Efficiency)
			t.Logf("%s run %d: valid=%v $%.2f/repo %.1f min/repo %v", c.ID, run, r.Grade.Valid, r.Efficiency.USDPerRepo, r.Efficiency.MinutesPerRepo, r.Grade.Problems)
		}
	}
	ec2 := EfficiencyProblems(eff)
	doc := map[string]any{"schema": "athel-evals/m5-set@1", "model": set.Model, "runs_per_case": set.Runs,
		"valid": valid, "runs": total, "cost_usd": cost, "efficiency_problems": ec2, "cases": results}
	p, err := evalkit.WriteHistory(filepath.Join("..", "..", "history"), "m5-set", started, tl, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d runs valid, $%.2f; history %s", valid, total, cost, p)
	if valid != total {
		t.Errorf("EC-1 not met: %d of %d runs valid", valid, total)
	}
	for _, e := range ec2 {
		t.Errorf("EC-2 not met: %s", e)
	}
}

// TestPrepareAndCollect runs the runner's free parts on a real sandbox, with no model: the
// seeds land on each repo's main and in the developer's clones, the system check is configured
// and passes on main, and a run that never started grades as not valid, with its problems.
func TestPrepareAndCollect(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fixture and the CLI")
	}
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := evalkit.BuildTools(repo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	sb, err := evalkit.NewSandbox(t.TempDir(), tl)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepare(sb, c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(sb.Layout.Repos[c.Planted.Repo].Clone, filepath.FromSlash(c.Planted.File)))
	if err != nil || !strings.Contains(string(b), c.Planted.Text) {
		t.Fatalf("the developer's clone has the seed: %v", err)
	}
	if failed := systemFromMain(sb); failed != "" {
		t.Fatalf("the system check on main: %s", failed)
	}
	if _, err := os.Stat(filepath.Join(sb.WS, ".athel", "config", "system.yaml")); err != nil {
		t.Fatal(err)
	}
	a, err := collect(sb, c, "")
	if err != nil {
		t.Fatal(err)
	}
	g := GradeRun(c, a)
	if g.Valid || !strings.Contains(strings.Join(g.Problems, "\n"), "not in_review") || a.MergeCalled {
		t.Errorf("a run that never started: %+v", g)
	}
}
