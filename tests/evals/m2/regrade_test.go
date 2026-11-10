package m2

import (
	"encoding/json"
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestRegrade grades a saved run again with the current cases and grader, without any model
// call: ATHEL_EVALS_REGRADE=../history/m2-<UTC>.json. Each run's claim block, ceilings and
// attempted commands are in the history file for this.
func TestRegrade(t *testing.T) {
	path := os.Getenv("ATHEL_EVALS_REGRADE")
	if path == "" {
		t.Skip("set ATHEL_EVALS_REGRADE to a history file")
	}
	sum, err := regrade(loadCases(t), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", sum)
}

func regrade(cases []Case, path string) (Summary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Summary{}, err
	}
	var doc struct {
		Cases map[string][]Result `json:"cases"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return Summary{}, err
	}
	results := map[string][]Result{}
	var kept []Case
	for _, c := range cases {
		saved, ok := doc.Cases[c.ID]
		if !ok {
			continue
		}
		kept = append(kept, c)
		for _, r := range saved {
			var a Answer
			if err := yaml.Unmarshal([]byte(r.Block), &a); err != nil {
				return Summary{}, err
			}
			r.Grade = GradeAnswer(c, a, r.Ceilings, r.Actions, r.Block)
			r.Pass = r.Error == "" && r.Grade.Pass()
			results[c.ID] = append(results[c.ID], r)
		}
	}
	return summarise(kept, results), nil
}

func TestRegradeSavedRun(t *testing.T) {
	dir := t.TempDir()
	block := "claims:\n  - {id: C1, subject: {kind: service, name: users}, predicate: owns_data, object: {kind: column, name: users.phone}, env: any, confidence: likely}\n"
	hist := map[string]any{"cases": map[string]any{
		"data-owner-clean": []map[string]any{{"run": 1, "claim_block": block, "ceilings": map[string]string{"C1": "likely"}}},
		"gone-case":        []map[string]any{{"run": 1}},
	}}
	b, _ := json.Marshal(hist)
	p := dir + "/h.json"
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := regrade(loadCases(t), p)
	if err != nil || sum.Runs != 1 || sum.CasesPassed != 1 || !sum.Met {
		t.Errorf("regrade: %+v %v", sum, err)
	}
	for _, bad := range []string{"{", `{"cases":{"data-owner-clean":[{"claim_block":"claims: {"}]}}`} {
		if err := os.WriteFile(p, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := regrade(loadCases(t), p); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
	if _, err := regrade(nil, dir+"/missing.json"); err == nil {
		t.Error("a missing file must fail")
	}
}
