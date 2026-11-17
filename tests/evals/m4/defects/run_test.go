package defects

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// The runner (M4-plan §4.10) prepares each case with the CLI alone, then runs one `claude -p`
// session that does what /athel:build's step 9 does: it hands the task to the plugin's real
// reviewer (and, for a security defect, the security agent too) with the same manifest, and
// nothing else. It makes paid model calls, so it runs only with ATHEL_EVALS=1 and the owner's
// go-ahead. Isolation and settings are evalkit's.

// Result is one graded run.
type Result struct {
	Run      int             `json:"run"`
	Grade    Grade           `json:"grade"`
	Session  evalkit.Session `json:"session"`
	Findings []Finding       `json:"findings"`
	Error    string          `json:"error,omitempty"`
}

// Prompt is the session's one prompt: the checkers of the build loop, from their manifest.
func Prompt(sb *evalkit.Sandbox, c Case) string {
	agents := "the `athel:reviewer` subagent"
	if c.Security {
		agents = "the `athel:reviewer` and `athel:security` subagents, in one message so they run in parallel"
	}
	return "Run `athel --json resume " + Change + "`. Then do step 9 of /athel:build for task " + Task + " of " + Change +
		": delegate to " + agents + ", giving each only the change " + Change + ", the task " + Task +
		", the worktree folder " + Worktree(sb, c) + " and the manifest path " + Manifest +
		". Do not review the code yourself, and stop when they return."
}

func runOne(t *testing.T, c Case, run int, tl evalkit.Tools, set evalkit.Settings) Result {
	t.Helper()
	res := Result{Run: run}
	sb, err := evalkit.NewSandbox(set.RunDir(t.TempDir(), c.ID, run), tl)
	if err == nil {
		err = Prepare(sb, c)
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), set.Timeout)
		res.Session, err = sb.RunClaude(ctx, evalkit.Prompt{Text: Prompt(sb, c), Transcript: "review.jsonl", Model: set.Model})
		cancel()
	}
	saved := false
	if sb != nil {
		var rerr error
		res.Findings, saved, rerr = ReadRound(sb.WS)
		if err == nil {
			err = rerr
		}
	}
	res.Grade = GradeRun(c, res.Findings, saved)
	if err != nil {
		res.Error = err.Error()
		res.Grade.Problems = append(res.Grade.Problems, "run failed: "+res.Error)
	}
	return res
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's go-ahead (M4-plan EC-2)")
	}
	set, err := evalkit.LoadSettings(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	tl := buildTools(t)
	if tl.Claude, err = exec.LookPath("claude"); err != nil {
		t.Fatal(err)
	}
	all, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var cases []Case
	for _, c := range all {
		if set.Selected(c.ID) {
			cases = append(cases, c)
		}
	}
	started := time.Now().UTC()
	results := map[string][]Result{}
	grades := map[string][]Grade{}
	cost := 0.0
	for _, c := range cases {
		for run := 1; run <= set.Runs; run++ {
			r := runOne(t, c, run, tl, set)
			results[c.ID], grades[c.ID] = append(results[c.ID], r), append(grades[c.ID], r.Grade)
			cost += r.Session.CostUSD
			t.Logf("%s run %d: caught=%v by %s, false alarms %d, findings %d, $%.2f %v", c.ID, run, r.Grade.Caught,
				r.Grade.MatchedBy, r.Grade.FalseAlarms, r.Grade.Findings, r.Session.CostUSD, r.Grade.Problems)
		}
	}
	sum := Summarise(cases, grades)
	doc := map[string]any{"schema": "athel-evals/m4-defects@1", "model": set.Model, "runs_per_case": set.Runs,
		"cost_usd": cost, "summary": sum, "cases": results}
	p, err := evalkit.WriteHistory(filepath.Join("..", "..", "history"), "m4-defects", started, tl, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("catch rate %.1f%% (%d/%d), false alarms on %d of %d clean runs, $%.2f; history %s",
		100*sum.CatchRate, sum.Caught, sum.DefectRuns, sum.FalseAlarmRuns, sum.CleanRuns, cost, p)
	if !sum.Met {
		t.Errorf("EC-2 not met: catch rate %.3f < %.2f; missed %v", sum.CatchRate, Bar, sum.Missed)
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

// TestDefectSandbox prepares a case in the eval sandbox with the CLI alone and saves the round a
// reviewer would, then grades it: Prepare, the manifest path, ReadRound and the grader agree
// with the files the CLI really writes, without a model.
func TestDefectSandbox(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fixture and three binaries")
	}
	all, err := Load()
	if err != nil || len(all) == 0 {
		t.Skipf("no corpus yet: %v", err)
	}
	c := all[0]
	sb, err := evalkit.NewSandbox(t.TempDir(), buildTools(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Prepare(sb, c); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sb.WS, filepath.FromSlash(Manifest))); err != nil {
		t.Fatalf("the reviewer's manifest: %v", err)
	}
	if p := Prompt(sb, c); !strings.Contains(p, Manifest) || !strings.Contains(p, Worktree(sb, c)) || strings.Contains(p, c.Planted.File) {
		t.Errorf("the prompt gives the manifest and the worktree and nothing about the defect: %s", p)
	}
	round := "verdict: changes_requested\nnew:\n  - {severity: major, source: reviewer, location: \"" + c.Planted.File + ":" +
		strconv.Itoa(c.Planted.Lines[0]) + "\", rule: " + c.Rules[0] + ", finding: f, required: r}\n"
	if _, err := sb.Athel(round, "--json", "--session", "eval-check", "resume", Change); err != nil {
		t.Fatal(err)
	}
	if _, err := sb.Athel(round, "--json", "--session", "eval-check", "review", "save", Change, Task, "-"); err != nil {
		t.Fatal(err)
	}
	fs, saved, err := ReadRound(sb.WS)
	if err != nil {
		t.Fatal(err)
	}
	if g := GradeRun(c, fs, saved); !g.Caught || g.MatchedBy != "R1-01" {
		t.Errorf("a finding on the planted lines with the defect's rule is caught: %+v", g)
	}
}
