package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// The runner (M4-plan §4.10) takes each PRD through Athel as a developer would, one `claude -p`
// session per prompt in the same sandbox: /athel:start, /athel:spec, the developer's
// /athel:decide with every card's recommended option (whenever cards are open), /athel:plan
// (once more if it ended on a card), /athel:approve, /athel:build (again while tasks remain, up to buildSessions), /athel:ship and
// /athel:triage with every finding set to ticket. It never answers anything else. Paid model
// calls: only with ATHEL_EVALS=1 and the owner's go-ahead.

const buildSessions = 4

// Result is one graded run.
type Result struct {
	Run       int               `json:"run"`
	Grade     Grade             `json:"grade"`
	Sessions  []evalkit.Session `json:"sessions"`
	Artifacts Artifacts         `json:"artifacts"`
	Error     string            `json:"error,omitempty"`
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
	ctx, cancel := context.WithTimeout(context.Background(), limit(name, r.set.Timeout))
	defer cancel()
	s, err := r.sb.RunClaude(ctx, evalkit.Prompt{Text: text, Transcript: fmt.Sprintf("%02d-%s.jsonl", r.n, name), Model: r.set.Model})
	r.res.Sessions = append(r.res.Sessions, s)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// limit is a session's time limit: a build session runs the developer, the gates and three
// checkers, round after round, so it gets three times the others' (trial 5 hit 20 minutes).
func limit(step string, base time.Duration) time.Duration {
	if step == "build" {
		return 3 * base
	}
	return base
}

// decide answers every open card with its recommendation, as the scripted developer.
func (r *runner) decide() error {
	d, err := r.athelJSON("decide", "list", Change, "--open")
	if err != nil {
		return err
	}
	if p := DecidePrompt(d); p != "" {
		return r.prompt("decide", p)
	}
	return nil
}

// DecidePrompt is the developer's /athel:decide with each open card's recommended option, or "".
func DecidePrompt(list map[string]any) string {
	ds, _ := list["decisions"].([]any)
	var answers []string
	for _, x := range ds {
		d, _ := x.(map[string]any)
		rec, _ := d["recommendation"].(map[string]any)
		if d["kind"] == "card" && rec["option"] != nil {
			answers = append(answers, fmt.Sprintf("%v=%v", d["id"], rec["option"]))
		}
	}
	if len(answers) == 0 {
		return ""
	}
	return "/athel:decide " + Change + " " + strings.Join(answers, " ")
}

func (r *runner) run(prd string) error {
	for _, phase := range []func(string) error{r.specify, r.build, r.ship} {
		if err := phase(prd); err != nil {
			return err
		}
	}
	return nil
}

// specify is start and spec, then plan and approve, each after any open cards are answered.
func (r *runner) specify(prd string) error {
	for _, st := range []struct{ name, text string }{{"start", "/athel:start " + prd}, {"spec", "/athel:spec " + Change}} {
		if err := r.prompt(st.name, st.text); err != nil {
			return err
		}
	}
	if err := r.decide(); err != nil {
		return err
	}
	// A plan step that raises a card ends without a plan and tells the developer to run
	// /athel:plan again once it is answered, which they do, once.
	for range 2 {
		if err := r.prompt("plan", "/athel:plan "+Change); err != nil {
			return err
		}
		if err := r.decide(); err != nil {
			return err
		}
		if _, err := r.athelJSON("plan", "summary", Change); err == nil {
			break
		}
	}
	// At CRITICAL risk the developer signs off the blast radius before approving (doc 05), as
	// the plan step tells them to.
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

// build runs /athel:build until every task is done, answering cards between sessions; a block
// with no card to answer ends the run.
func (r *runner) build(string) error {
	for i := 0; i < buildSessions && r.state() != "ready_for_pr" && r.state() != "in_review"; i++ {
		open, err := r.athelJSON("decide", "list", Change, "--open")
		if err != nil {
			return err
		}
		if r.state() == "blocked" && DecidePrompt(open) == "" {
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

// ship runs /athel:ship, the developer's /athel:triage when a finding is undecided, and ship
// again if the change is not in review yet.
func (r *runner) ship(string) error {
	if err := r.prompt("ship", "/athel:ship "+Change); err != nil {
		return err
	}
	f, err := r.athelJSON("finding", "list", Change)
	if err != nil {
		return err
	}
	if hasUndecided(f) {
		if err := r.prompt("triage", "/athel:triage "+Change+" all=ticket"); err != nil {
			return err
		}
	}
	if r.state() != "in_review" {
		return r.prompt("ship", "/athel:ship "+Change)
	}
	return nil
}

// hasUndecided reports whether `finding list` shows a finding the developer has not decided.
func hasUndecided(list map[string]any) bool {
	fs, _ := list["findings"].([]any)
	for _, x := range fs {
		if f, _ := x.(map[string]any); f["disposition"] == nil {
			return true
		}
	}
	return false
}

func runOne(t *testing.T, c Case, run int, tl evalkit.Tools, set evalkit.Settings) Result {
	t.Helper()
	res := Result{Run: run, Sessions: []evalkit.Session{}}
	dir := set.RunDir(t.TempDir(), c.ID, run)
	sb, err := evalkit.NewSandbox(dir, tl)
	before := ""
	if err == nil {
		err = seedSandbox(sb, c)
	}
	if err == nil {
		before, err = TreeHash(filepath.Join(sb.Home, "Developer"))
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
	res.Grade = GradeRun(c, res.Artifacts)
	if err != nil {
		res.Error = err.Error()
		res.Grade.Valid, res.Grade.Problems = false, append(res.Grade.Problems, "run failed: "+res.Error)
	}
	return res
}

// remoteOf is the case repo's fixture remote, as the developer's clone knows it.
func remoteOf(sb *evalkit.Sandbox, c Case) (string, error) {
	out, err := gitEnv(sb, filepath.Join(sb.Home, "Developer", c.Repo), "remote", "get-url", "origin")
	return strings.TrimSpace(out), err
}

func gitEnv(sb *evalkit.Sandbox, dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, []string{"HOME=" + sb.Home, "PATH=" + sb.SessionPath()}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return string(out), nil
}

// seedSandbox pushes the case's seed to the repo's remote main through the sandbox's own git
// mapping, then brings the developer's clone up to date, before the clones are hashed.
func seedSandbox(sb *evalkit.Sandbox, c Case) error {
	if len(c.Seed) == 0 {
		return nil
	}
	url, err := remoteOf(sb, c)
	if err != nil {
		return err
	}
	work := filepath.Join(sb.Dir, "seed-"+c.Repo)
	if _, err := gitEnv(sb, sb.Dir, "clone", "-q", url, work); err != nil {
		return err
	}
	run := func(dir string, args ...string) (string, error) { return gitEnv(sb, dir, args...) }
	if err := Seed(work, c, run); err != nil {
		return err
	}
	_, err = gitEnv(sb, filepath.Join(sb.Home, "Developer", c.Repo), "pull", "-q", "--ff-only")
	return err
}

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, v)
}

// collect reads the run's files and re-checks its result: it clones the pushed branch and runs
// the gates itself.
func collect(sb *evalkit.Sandbox, c Case, before string) (Artifacts, error) {
	var a Artifacts
	dir := filepath.Join(sb.WS, ".athel", "changes", Change)
	var ch struct {
		State string `yaml:"state"`
		Repos []struct {
			Name, Branch string
			BaseHead     string `yaml:"base_head"`
			Pushed       bool
		} `yaml:"repos"`
	}
	_ = readYAML(filepath.Join(dir, "change.yaml"), &ch) // a run that never started has none
	a.State = ch.State
	var pl struct{ Tasks []struct{ Kind string } }
	_ = readYAML(filepath.Join(dir, "plan.yaml"), &pl)
	for _, t := range pl.Tasks {
		a.Kinds = append(a.Kinds, t.Kind)
	}
	a.AllowedLines = allowedLines(filepath.Join(dir, "decisions.yaml"), c.Repo)
	var fs struct{ Findings []struct{ Location string } }
	_ = readYAML(filepath.Join(dir, "findings.yaml"), &fs)
	for _, f := range fs.Findings {
		a.Findings = append(a.Findings, f.Location)
	}
	var err error
	if a.Calls, err = ReadCalls(sb.GHState); err != nil {
		return a, err
	}
	if b, err := os.ReadFile(filepath.Join(sb.GHState, "prs.json")); err == nil {
		_ = json.Unmarshal(b, &a.PRs)
	}
	if a.Actions, err = sb.Actions(); err != nil {
		return a, err
	}
	after, err := TreeHash(filepath.Join(sb.Home, "Developer"))
	a.ClonesUnchanged = err == nil && after == before
	_, lerr := sb.Athel("", "--json", "git", "lint", Change, c.Repo)
	if a.LintOK = lerr == nil; lerr != nil {
		a.LintProblem = firstLines(lerr.Error(), 3)
	}
	for _, r := range ch.Repos {
		if r.Name == c.Repo && r.Pushed {
			a.Pushed = true
			return a, recheck(sb, c, r.Branch, r.BaseHead, &a)
		}
	}
	return a, nil
}

// allowedLines is the PR size the developer allowed for repo by answering its size card with
// "a" (ADR 028), or 0.
func allowedLines(path, repo string) int {
	var f struct {
		Decisions []struct {
			Question string `yaml:"question"`
			Options  []struct{ Key, Label string }
			Answer   *struct{ Option string }
		} `yaml:"decisions"`
	}
	if readYAML(path, &f) != nil {
		return 0
	}
	allowed := 0
	for _, d := range f.Decisions {
		if d.Answer == nil || d.Answer.Option != "a" || !strings.HasPrefix(d.Question, "The "+repo+" PR is over git.yaml's limit") {
			continue
		}
		for _, o := range d.Options {
			var n int
			if _, err := fmt.Sscanf(o.Label, "Allow up to %d changed lines in this PR", &n); err == nil && o.Key == "a" {
				allowed = max(allowed, n)
			}
		}
	}
	return allowed
}

// recheck clones the pushed branch fresh and grades it: the gates, its size and the planted bug.
func recheck(sb *evalkit.Sandbox, c Case, branch, base string, a *Artifacts) error {
	url, err := remoteOf(sb, c)
	if err != nil {
		return err
	}
	tree := filepath.Join(sb.Dir, "recheck-"+c.Repo)
	if _, err := gitEnv(sb, sb.Dir, "clone", "-q", "--branch", branch, url, tree); err != nil {
		return err
	}
	a.GateProblems = RerunGates(tree, base, sb.Env())
	if a.Lines, err = ChangedLines(tree, base, "HEAD"); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(tree, filepath.FromSlash(c.Planted.File)))
	a.PlantedKept = err == nil && strings.Contains(string(b), c.Planted.Text)
	return nil
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's go-ahead (M4-plan EC-1)")
	}
	set, err := evalkit.LoadSettings(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	tl := buildTools(t)
	if tl.Claude, err = exec.LookPath("claude"); err != nil {
		t.Fatal(err)
	}
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	results := map[string][]Result{}
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
			t.Logf("%s run %d: valid=%v %v", c.ID, run, r.Grade.Valid, r.Grade.Problems)
		}
	}
	doc := map[string]any{"schema": "athel-evals/m4-build@1", "model": set.Model, "runs_per_case": set.Runs,
		"valid": valid, "runs": total, "cost_usd": cost, "cases": results}
	p, err := evalkit.WriteHistory(filepath.Join("..", "..", "history"), "m4-build", started, tl, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d runs valid, $%.2f; history %s", valid, total, cost, p)
	if valid != total {
		t.Errorf("EC-1 not met: %d of %d runs valid", valid, total)
	}
}

// buildTools builds the CLI and the fixture's fakes from this repository.
func buildTools(t *testing.T) evalkit.Tools {
	t.Helper()
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := evalkit.BuildTools(repo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return tl
}
