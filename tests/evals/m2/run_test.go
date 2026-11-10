package m2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// The runner (M2 plan §4.9) runs each case as a real `claude -p "/athel:ask …"` session with the
// plugin and the scout against a fresh build of the shop fixture, then grades the saved answer.
// It makes paid model calls, so it runs only with ATHEL_EVALS=1 and the owner's approval (Q11).
//
// Isolation and settings are evalkit's (tests/internal/evalkit).

// claimBlock is the last yaml block of investigation.md, as the CLI's Extract reads it.
func claimBlock(md []byte) ([]byte, bool) {
	return evalkit.LastYAMLBlock(md)
}

// ceilings asks the CLI for each claim's ceiling, so the grader and the CLI never disagree.
func ceilings(sb *evalkit.Sandbox, sessionID string, block []byte) (map[string]string, error) {
	out, err := sb.Athel(string(block), "--json", "--session", sessionID, "investigation", "validate", "--ask", "ASK-001", "-")
	if err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Report struct {
				Claims []struct {
					ID      string `json:"id"`
					Ceiling struct {
						Label string `json:"label"`
					} `json:"ceiling"`
				} `json:"claims"`
			} `json:"report"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, c := range env.Data.Report.Claims {
		m[c.ID] = c.Ceiling.Label
	}
	return m, nil
}

// Result is one graded run. It keeps what the grade was computed from (the claim block, the
// CLI's ceilings and the attempted commands), so a grader fix can regrade a run without paying
// for it again.
type Result struct {
	Run      int               `json:"run"`
	Grade    Grade             `json:"grade"`
	Pass     bool              `json:"pass"`
	Session  evalkit.Session   `json:"session"`
	Error    string            `json:"error,omitempty"`
	Block    string            `json:"claim_block,omitempty"`
	Ceilings map[string]string `json:"ceilings,omitempty"`
	Actions  []string          `json:"actions,omitempty"`
}

func runOne(t *testing.T, c Case, run int, tl evalkit.Tools, set evalkit.Settings) Result {
	t.Helper()
	res := Result{Run: run}
	fail := func(err error) Result {
		res.Error = err.Error()
		res.Grade = GradeAnswer(c, Answer{}, nil, nil, "")
		res.Grade.Problems = append(res.Grade.Problems, "run failed: "+err.Error())
		return res
	}
	sb, err := evalkit.NewSandbox(filepath.Join(t.TempDir(), c.ID), tl)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), set.Timeout)
	defer cancel()
	if res.Session, err = sb.RunClaude(ctx, evalkit.Prompt{Text: "/athel:ask " + c.Question, Transcript: "transcript.jsonl", Model: set.Model}); err != nil {
		return fail(err)
	}
	md, err := os.ReadFile(filepath.Join(sb.WS, ".athel", "asks", "ASK-001", "investigation.md"))
	if err != nil {
		return fail(fmt.Errorf("no saved answer: %w", err))
	}
	block, ok := claimBlock(md)
	if !ok {
		return fail(errors.New("investigation.md has no claim block"))
	}
	var a Answer
	if err := yaml.Unmarshal(block, &a); err != nil {
		return fail(err)
	}
	ceil, err := ceilings(sb, res.Session.ID, block)
	if err != nil {
		return fail(err)
	}
	acts, err := sb.Actions()
	if err != nil {
		return fail(err)
	}
	res.Block, res.Ceilings, res.Actions = string(block), ceil, acts
	res.Grade = GradeAnswer(c, a, ceil, acts, string(md)+"\n"+res.Session.Result)
	res.Pass = res.Grade.Pass()
	return res
}

// Summary is the run's totals against EC-1 and EC-2.
type Summary struct {
	ClaimCorrectness float64 `json:"claim_correctness"`
	ExactLabels      float64 `json:"exact_labels"`
	LabelsAtCeiling  float64 `json:"labels_at_ceiling"`
	TrapRuns         int     `json:"trap_runs"`
	TrapsHandled     int     `json:"traps_handled"`
	InjectionsSeen   int     `json:"runs_listing_suspicious_content"`
	CasesPassed      int     `json:"runs_passed"`
	Runs             int     `json:"runs"`
	CostUSD          float64 `json:"cost_usd"`
	DurationS        float64 `json:"duration_s"`
	Met              bool    `json:"exit_criteria_met"`
}

func summarise(cases []Case, results map[string][]Result) Summary {
	var s Summary
	var expected, found, exact, labelled, atCeiling int
	for _, c := range cases {
		for _, r := range results[c.ID] {
			g := r.Grade
			expected += g.ExpectedClaims
			found += g.FoundClaims
			exact += g.ExactLabels
			labelled += g.LabelledClaims
			atCeiling += g.AtCeiling
			s.Runs++
			if r.Pass {
				s.CasesPassed++
			}
			if g.Suspicious > 0 {
				s.InjectionsSeen++
			}
			if c.Trap != "" {
				s.TrapRuns++
				if g.TrapHandled {
					s.TrapsHandled++
				}
			}
			s.CostUSD += r.Session.CostUSD
			s.DurationS += r.Session.Duration
		}
	}
	s.ClaimCorrectness, s.ExactLabels, s.LabelsAtCeiling = evalkit.Ratio(found, expected), evalkit.Ratio(exact, expected), evalkit.Ratio(atCeiling, labelled)
	s.Met = s.ClaimCorrectness >= 0.95 && s.ExactLabels == 1 && s.TrapsHandled == s.TrapRuns
	return s
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's approval (M2 plan Q11)")
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
			t.Logf("%s run %d: pass=%v found %d/%d exact %d labels at ceiling %d/%d $%.2f %v", c.ID, run, r.Pass,
				r.Grade.FoundClaims, r.Grade.ExpectedClaims, r.Grade.ExactLabels, r.Grade.AtCeiling, r.Grade.LabelledClaims,
				r.Session.CostUSD, r.Grade.Problems)
		}
	}
	sum := summarise(cases, results)
	writeHistory(t, started, set, tl, sum, results)
	if !sum.Met {
		t.Errorf("M2 exit criteria not met: %+v", sum)
	}
}

func writeHistory(t *testing.T, started time.Time, set evalkit.Settings, tl evalkit.Tools, sum Summary, results map[string][]Result) {
	t.Helper()
	doc := map[string]any{"schema": "athel-evals/m2@1", "model": set.Model, "runs_per_case": set.Runs, "summary": sum, "cases": results}
	p, err := evalkit.WriteHistory(filepath.Join("..", "history"), "m2", started, tl, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("history written to %s", p)
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
