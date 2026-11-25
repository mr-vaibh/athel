package set

import (
	"os"
	"strings"
	"testing"
)

func TestLoadCases(t *testing.T) {
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3 {
		t.Fatalf("cases = %d", len(cases))
	}
	three := false
	for _, c := range cases {
		if _, err := os.Stat(c.PRD); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
		if c.Planted.Repo == "" || c.Seed[c.Planted.Repo] == nil || !strings.Contains(c.Seed[c.Planted.Repo][c.Planted.File], c.Planted.Text) {
			t.Errorf("%s: the planted bug is not in its seed", c.ID)
		}
		three = three || len(c.Repos) == 3
	}
	if !three {
		t.Error("the exit criterion needs a 3-service case")
	}
}

func label(name string) []struct {
	Name string `json:"name"`
} {
	return []struct {
		Name string `json:"name"`
	}{{Name: name}}
}

func good() (Case, Artifacts) {
	yes := true
	c := Case{ID: "x", Repos: []string{"users", "orders"}, Strategy: "expand_contract", Deferred: &yes, Kinds: []string{"expand", "consume"},
		Planted: Planted{Repo: "orders", File: "clients/backoff.go"}}
	body := func(other string) string {
		return "This PR is step 1 of 2 in the merge order of CHG-001 (2 PRs). " + other
	}
	a := Artifacts{State: "in_review", Kinds: []string{"expand", "consume", "contract"}, Strategy: "expand_contract",
		Set:       []Entry{{Repo: "users", Order: 1}, {Repo: "orders", Order: 2}, {Repo: "users", Order: 3, Deferred: true}},
		Rehearsal: "pass",
		PRs: []PR{{Repo: "acme/users", Number: 901, Body: body("acme/orders#902"), Labels: label("athel:CHG-001")},
			{Repo: "acme/orders", Number: 902, Body: body("acme/users#901"), Labels: label("athel:CHG-001")}},
		DraftThenReady: map[string]bool{"acme/users": true, "acme/orders": true},
		Findings:       []string{"clients/backoff.go:7"}, PlantedKept: true, ClonesUnchanged: true,
		Cards: []string{"Which format for users?", "Allow 600 lines?"}}
	return c, a
}

func TestGradeRun(t *testing.T) {
	if g := GradeRun(good()); !g.Valid {
		t.Fatalf("a good run: %v", g.Problems)
	}
	c, a := good()
	c.Deferred, a.Set = nil, a.Set[:2]
	if g := GradeRun(c, a); !g.Valid {
		t.Errorf("a case that takes the cleanup either way: %v", g.Problems)
	}
	tests := []struct {
		name   string
		mutate func(*Artifacts)
		want   string
	}{
		{"not in review", func(a *Artifacts) { a.State = "building" }, "not in_review"},
		{"a kind missing", func(a *Artifacts) { a.Kinds = []string{"expand"} }, "no consume task"},
		{"a repo missing from the set", func(a *Artifacts) { a.Set = a.Set[:1] }, "the change set holds [users]"},
		{"the wrong strategy", func(a *Artifacts) { a.Strategy = "coordinated" }, "strategy is coordinated"},
		{"no cleanup", func(a *Artifacts) { a.Set = a.Set[:2] }, "deferred cleanup is false"},
		{"no rehearsal", func(a *Artifacts) { a.Rehearsal = "" }, "rehearsal did not pass"},
		{"a PR missing", func(a *Artifacts) { a.PRs = a.PRs[:1] }, "no PR in acme/orders"},
		{"not draft first", func(a *Artifacts) { a.DraftThenReady["acme/users"] = false }, "not opened as a draft"},
		{"no label", func(a *Artifacts) { a.PRs[0].Labels = nil }, "does not carry the label"},
		{"no place", func(a *Artifacts) { a.PRs[0].Body = "acme/orders#902" }, "does not give its place"},
		{"no link", func(a *Artifacts) { a.PRs[0].Body = "merge order of CHG-001" }, "does not link acme/orders#902"},
		{"a merge by Athel", func(a *Artifacts) { a.MergeCalled = true }, "a merge was attempted by Athel"},
		{"red gates", func(a *Artifacts) { a.GateProblems = map[string][]string{"users": {"unit: FAIL"}} }, "users: unit: FAIL"},
		{"lint", func(a *Artifacts) { a.LintProblems = map[string]string{"orders": "no trailer"} }, "orders: the branch or a commit fails"},
		{"a broken main", func(a *Artifacts) { a.MergeProblems = []string{"after acme/users#901: orders flow failed"} }, "after acme/users#901"},
		{"clones changed", func(a *Artifacts) { a.ClonesUnchanged = false }, "clones changed"},
		{"no side finding", func(a *Artifacts) { a.Findings = nil }, "no side finding names"},
		{"the planted bug edited", func(a *Artifacts) { a.PlantedKept = false }, "was edited"},
		{"a question asked twice", func(a *Artifacts) { a.Cards = append(a.Cards, "Which format for orders?") }, "asked twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, a := good()
			tt.mutate(&a)
			g := GradeRun(c, a)
			if g.Valid || !strings.Contains(strings.Join(g.Problems, "\n"), tt.want) {
				t.Errorf("problems %q, want %q", g.Problems, tt.want)
			}
		})
	}
}

func TestEfficiency(t *testing.T) {
	if e := Measure(3, 24, 3*20*60); e.USDPerRepo != 8 || e.MinutesPerRepo != 20 {
		t.Errorf("measure: %+v", e)
	}
	if e := Measure(0, 5, 60); e != (Efficiency{}) {
		t.Errorf("no repos: %+v", e)
	}
	if Median(nil) != 0 || Median([]float64{3, 1, 2}) != 2 || Median([]float64{4, 1, 3, 2}) != 2.5 {
		t.Error("median")
	}
	if p := EfficiencyProblems([]Efficiency{{Repos: 3, USDPerRepo: 9, MinutesPerRepo: 22}, {Repos: 2, USDPerRepo: 8, MinutesPerRepo: 18}}); len(p) != 0 {
		t.Errorf("within the bar: %v", p)
	}
	p := EfficiencyProblems([]Efficiency{{Repos: 3, USDPerRepo: 12, MinutesPerRepo: 30}, {}})
	if len(p) != 2 || !strings.Contains(p[0], "median cost per repo $12.00") || !strings.Contains(p[1], "30.0 min") {
		t.Errorf("over the bar: %v", p)
	}
}

func TestDuplicates(t *testing.T) {
	got := Duplicates([]string{"Which format for users?", "Which FORMAT for gateway?", "Allow more lines?", "Which format for orders?"})
	if len(got) != 1 || got[0] != "Which FORMAT for gateway?" {
		t.Errorf("duplicates = %v", got)
	}
}
