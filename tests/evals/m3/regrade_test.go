package m3

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRegrade grades a saved run again with the current cases and grader, without any model
// call: ATHEL_EVALS_REGRADE=../history/m3-<UTC>.json. Each run's artifacts are in the file.
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
			r.Grade = GradeRun(c, r.Artifacts)
			r.Pass = r.Error == "" && r.Grade.Pass()
			results[c.ID] = append(results[c.ID], r)
		}
	}
	return summarise(kept, results), nil
}

func TestRegradeSavedRun(t *testing.T) {
	dir := t.TempDir()
	a := Artifacts{State: "planned", Trace: "# Traceability · CHG-001 · complete\n", Blast: sampleBlast, Plan: "tasks:\n  - {kind: migrate}\n"}
	hist := map[string]any{"cases": map[string]any{
		"phone-e164": []Result{{Run: 1, Artifacts: a}, {Run: 2, Artifacts: a, Error: "spec: timed out"}},
		"gone-case":  []Result{{Run: 1}},
	}}
	b, _ := json.Marshal(hist)
	p := filepath.Join(dir, "h.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := regrade(loadCases(t), p)
	if err != nil || sum.Runs != 2 || sum.Valid != 1 || sum.Met {
		t.Errorf("regrade: %+v %v", sum, err)
	}
	if err := os.WriteFile(p, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := regrade(loadCases(t), p); err == nil {
		t.Error("a broken file must fail")
	}
	if _, err := regrade(nil, filepath.Join(dir, "missing.json")); err == nil {
		t.Error("a missing file must fail")
	}
}
