package m3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// The runner (M3 plan §4.9) takes each case through M3 as a developer would, one `claude -p`
// session per prompt in the same sandbox: /athel:start with the PRD file, /athel:spec, the
// developer's /athel:decide with every card's recommended option, then /athel:plan. Locks are
// released when a session ends, and the decide grant comes from that session's own prompt. It
// makes paid model calls, so it runs only with ATHEL_EVALS=1 and the owner's go-ahead (M3 plan
// Q1). Isolation and settings are evalkit's.

// change is the ID /athel:start gives the first change of a fresh workspace.
const change = "CHG-001"

// Result is one graded run, with the artifacts it was graded from.
type Result struct {
	Run       int               `json:"run"`
	Grade     Grade             `json:"grade"`
	Pass      bool              `json:"pass"`
	Sessions  []evalkit.Session `json:"sessions"`
	Error     string            `json:"error,omitempty"`
	Artifacts Artifacts         `json:"artifacts"`
}

// step is one prompt of a run; prompt returns "" when the step has nothing to do.
type step struct {
	name   string
	prompt func(sb *evalkit.Sandbox) (string, error)
}

func steps(prdPath string) []step {
	fixed := func(text string) func(*evalkit.Sandbox) (string, error) {
		return func(*evalkit.Sandbox) (string, error) { return text, nil }
	}
	return []step{
		{"start", fixed("/athel:start " + prdPath)},
		{"spec", fixed("/athel:spec " + change)},
		{"decide", func(sb *evalkit.Sandbox) (string, error) {
			out, err := sb.Athel("", "--json", "decide", "list", change, "--open")
			if err != nil {
				return "", err
			}
			return decidePrompt(out)
		}},
		{"plan", fixed("/athel:plan " + change)},
	}
}

// decidePrompt is what the scripted developer types: every open card answered with its
// recommended option, or "" when no card is open.
func decidePrompt(list []byte) (string, error) {
	var env struct {
		Data struct {
			Decisions []struct {
				ID             string `json:"id"`
				Kind           string `json:"kind"`
				Recommendation struct {
					Option string `json:"option"`
				} `json:"recommendation"`
			} `json:"decisions"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list, &env); err != nil {
		return "", fmt.Errorf("decide list: %w", err)
	}
	var answers []string
	for _, d := range env.Data.Decisions {
		if d.Kind != "card" {
			continue
		}
		if d.Recommendation.Option == "" {
			return "", fmt.Errorf("card %s has no recommended option", d.ID)
		}
		answers = append(answers, d.ID+"="+d.Recommendation.Option)
	}
	if len(answers) == 0 {
		return "", nil
	}
	return "/athel:decide " + change + " " + strings.Join(answers, " "), nil
}

func runOne(t *testing.T, c Case, run int, tl evalkit.Tools, set evalkit.Settings) Result {
	t.Helper()
	res := Result{Run: run, Sessions: []evalkit.Session{}}
	sb, err := evalkit.NewSandbox(filepath.Join(t.TempDir(), c.ID), tl)
	if err == nil {
		err = runSteps(sb, c, set, &res)
	}
	if err != nil {
		res.Error = err.Error()
	}
	if sb != nil {
		res.Artifacts, err = collect(sb.WS)
		if err != nil && res.Error == "" {
			res.Error = err.Error()
		}
	}
	res.Grade = GradeRun(c, res.Artifacts)
	if res.Error != "" {
		res.Grade.Problems = append(res.Grade.Problems, "run failed: "+res.Error)
	}
	res.Pass = res.Error == "" && res.Grade.Pass()
	return res
}

// runSteps runs the case's prompts in order. A step that fails stops the run; what the earlier
// steps saved is still graded.
func runSteps(sb *evalkit.Sandbox, c Case, set evalkit.Settings, res *Result) error {
	prdPath := filepath.Join(sb.Dir, "prd.md")
	b, err := os.ReadFile(c.PRD)
	if err != nil {
		return err
	}
	if err := os.WriteFile(prdPath, b, 0o600); err != nil {
		return err
	}
	for i, st := range steps(prdPath) {
		text, err := st.prompt(sb)
		if err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
		if text == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), set.Timeout)
		s, err := sb.RunClaude(ctx, evalkit.Prompt{Text: text, Transcript: fmt.Sprintf("%d-%s.jsonl", i+1, st.name), Model: set.Model})
		cancel()
		res.Sessions = append(res.Sessions, s)
		if err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
	}
	return nil
}

// collect reads what the run left in the change folder; a file the run never wrote stays empty.
func collect(ws string) (Artifacts, error) {
	dir := filepath.Join(ws, ".athel", "changes", change)
	read := func(name string) (string, error) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return string(b), err
	}
	var a Artifacts
	for _, f := range []struct {
		dst  *string
		name string
	}{{&a.Review, "prd/review-v1.md"}, {&a.Decisions, "decisions.yaml"}, {&a.Blast, "blast-radius.md"}, {&a.Plan, "plan.yaml"}, {&a.Trace, "traceability.md"}} {
		var err error
		if *f.dst, err = read(f.name); err != nil {
			return a, err
		}
	}
	meta, err := read("change.yaml")
	if err != nil {
		return a, err
	}
	var c struct {
		State string `yaml:"state"`
	}
	if err := yaml.Unmarshal([]byte(meta), &c); err != nil {
		return a, fmt.Errorf("change.yaml: %w", err)
	}
	a.State = c.State
	return a, nil
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's go-ahead (M3 plan Q1)")
	}
	set, err := evalkit.LoadSettings(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	tl := buildTools(t)
	if tl.Claude, err = exec.LookPath("claude"); err != nil {
		t.Fatal(err)
	}
	var cases []Case
	for _, c := range loadCases(t) {
		if set.Selected(c.ID) {
			cases = append(cases, c)
		}
	}
	started := time.Now().UTC()
	results := map[string][]Result{}
	for _, c := range cases {
		for run := 1; run <= set.Runs; run++ {
			r := runOne(t, c, run, tl, set)
			results[c.ID] = append(results[c.ID], r)
			t.Logf("%s run %d: pass=%v risk %s found %d/%d cards %d %v", c.ID, run, r.Pass, r.Grade.Risk,
				r.Grade.Found, r.Grade.Expected, r.Grade.Cards, r.Grade.Problems)
		}
	}
	sum := summarise(cases, results)
	doc := map[string]any{"schema": "athel-evals/m3@1", "model": set.Model, "runs_per_case": set.Runs, "summary": sum, "cases": results}
	p, err := evalkit.WriteHistory(filepath.Join("..", "history"), "m3", started, tl, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("history written to %s", p)
	if !sum.Met {
		t.Errorf("M3 exit criteria not met: %+v", sum)
	}
}

// buildTools builds the CLI and the fixture's fakes from this repository.
func buildTools(t *testing.T) evalkit.Tools {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := evalkit.BuildTools(repo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return tl
}
