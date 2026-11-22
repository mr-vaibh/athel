package review

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func newFinding(sev, loc string) Draft {
	return Draft{Severity: sev, Source: "reviewer", Location: loc, Rule: "errors.no-silent-catch", Finding: "x", Required: "y"}
}

func TestFirstRoundGivesIDs(t *testing.T) {
	r, err := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3"), newFinding(Nit, "src/b.go:9")}}, "abc", MaxRounds)
	if err != nil {
		t.Fatal(err)
	}
	if r.Round != 1 || r.Findings[0].ID != "R1-01" || r.Findings[1].ID != "R1-02" || r.Findings[0].Status != Open || len(r.Findings[0].History) != 1 {
		t.Errorf("%+v", r)
	}
}

func TestLaterRoundsCarryEveryOpenFinding(t *testing.T) {
	r1, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3"), newFinding(Minor, "src/b.go:9")}}, "h1", MaxRounds)
	if err := r1.Fix("R1-01", "c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Next(r1, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Verified}}}, "h2", MaxRounds); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "R1-02") {
		t.Errorf("a round that drops an open finding: %v", err)
	}
	r2, err := Next(r1, Submission{Verdict: ChangesRequested, Previous: []Resolution{{ID: "R1-01", Status: Verified}, {ID: "R1-02", Status: Open, Note: "still"}},
		New: []Draft{newFinding(Blocker, "src/c.go:1")}}, "h2", MaxRounds)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Round != 2 || len(r2.Findings) != 3 || r2.Findings[0].Status != Verified || r2.Findings[2].ID != "R2-01" || len(r2.Findings[0].History) != 3 {
		t.Errorf("%+v", r2.Findings)
	}
	if open := r2.Blocking(); strings.Join(open, ",") != "R2-01" {
		t.Errorf("an open blocker blocks, a minor does not: %v", open)
	}
	r3, err := Next(r2, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-02", Status: Open}, {ID: "R2-01", Status: Verified}}}, "h3", MaxRounds)
	if err == nil || !strings.Contains(err.Error(), "R2-01 is open, not fixed") {
		t.Errorf("only a fixed finding can be verified: %+v %v", r3, err)
	}
}

func TestDeveloperActions(t *testing.T) {
	r, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3")}}, "h1", MaxRounds)
	for _, err := range []error{r.Fix("R9-09", "c"), r.Dispute("R9-09", "why"), r.Dispute("R1-01", " "), r.Fix("R1-01", "")} {
		if !aerr.HasCode(err, aerr.StateInvalid) && !aerr.HasCode(err, aerr.StateNotFound) {
			t.Errorf("refused: %v", err)
		}
	}
	if err := r.Dispute("R1-01", "the caller already validates"); err != nil || r.Findings[0].Status != Disputed || r.Findings[0].Dispute != "the caller already validates" {
		t.Errorf("%+v %v", r.Findings[0], err)
	}
	if r.Disputes("R1-01") != 1 {
		t.Errorf("disputes = %d", r.Disputes("R1-01"))
	}
}

func TestDisputedTwiceAndRoundLimit(t *testing.T) {
	r, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3")}}, "h1", MaxRounds)
	_ = r.Dispute("R1-01", "no")
	r2, _ := Next(r, Submission{Verdict: ChangesRequested, Previous: []Resolution{{ID: "R1-01", Status: Open, Note: "still a bug"}}}, "h2", MaxRounds)
	_ = r2.Dispute("R1-01", "still no")
	if r2.Disputes("R1-01") != 2 || strings.Join(r2.DisputedTwice(), ",") != "R1-01" {
		t.Errorf("two rounds of dispute: %d %v", r2.Disputes("R1-01"), r2.DisputedTwice())
	}
	last := r2
	for i := 3; i <= MaxRounds; i++ {
		var err error
		if last, err = Next(last, Submission{Verdict: ChangesRequested, Previous: []Resolution{{ID: "R1-01", Status: Open}}}, "h", MaxRounds); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Next(last, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Open}}}, "h", MaxRounds); !aerr.HasCode(err, aerr.BudgetRounds) {
		t.Errorf("a sixth round: %v", err)
	}
	if r6, err := Next(last, Submission{Verdict: ChangesRequested, Previous: []Resolution{{ID: "R1-01", Status: Open}}}, "h", 2*MaxRounds); err != nil || r6.Round != MaxRounds+1 {
		t.Errorf("a sixth round once more rounds are allowed: %v %v", r6, err)
	}
}

func TestDone(t *testing.T) {
	r, _ := Next(nil, Submission{Verdict: Approved, New: []Draft{newFinding(Minor, "src/a.go:3")}}, "h1", MaxRounds)
	if !r.Done() {
		t.Error("approved with only a minor finding is done")
	}
	r3, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3")}}, "h1", MaxRounds)
	if r3.Done() {
		t.Error("an open major finding is never done")
	}
	r3.Findings[0].Status, r3.Verdict = Waived, Approved // the next round, after the developer's waiver
	if !r3.Done() {
		t.Error("a waived major is done")
	}
	r3.Findings[0].Severity, r3.Findings[0].Status = Blocker, Waived
	if r3.Done() {
		t.Error("a blocker can never be waived (doc 06 severity table)")
	}
}

func TestSubmissionChecks(t *testing.T) {
	bad := []Submission{
		{Verdict: "maybe"},
		{Verdict: Approved, New: []Draft{{Severity: "huge", Source: "reviewer", Location: "a", Rule: "r", Finding: "f", Required: "q"}}},
		{Verdict: Approved, New: []Draft{{Severity: Major, Source: "intern", Location: "a", Rule: "r", Finding: "f", Required: "q"}}},
		{Verdict: Approved, New: []Draft{{Severity: Major, Source: "reviewer", Rule: "r", Finding: "f", Required: "q"}}},
		{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Verified}}},
		{Verdict: Approved, New: []Draft{{Severity: Major, Source: "reviewer", Location: "a", Rule: "style.tabs", Finding: "f", Required: "q"}}},
	}
	for i, s := range bad {
		if _, err := Next(nil, s, "h", MaxRounds); err == nil {
			t.Errorf("submission %d: want an error", i)
		}
	}
	r, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3")}}, "h1", MaxRounds)
	if _, err := Next(r, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Waived}}}, "h2", MaxRounds); err == nil {
		t.Error("only a human waives (task 09), never the reviewer")
	}
}

func TestLocationFile(t *testing.T) {
	for loc, want := range map[string]string{"src/a.go:42-58": "src/a.go", "src/a.go": "src/a.go", "web/x.ts:3": "web/x.ts"} {
		if got := LocationFile(loc); got != want {
			t.Errorf("%s: %q", loc, got)
		}
	}
}

func TestUnknownEarlierFinding(t *testing.T) {
	r, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3")}}, "h1", MaxRounds)
	if _, err := Next(r, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Open}, {ID: "R1-99", Status: Open}}}, "h2", MaxRounds); err == nil || !strings.Contains(err.Error(), "no earlier finding") {
		t.Errorf("%v", err)
	}
	if r.Disputes("R9-09") != 0 {
		t.Error("no such finding, no disputes")
	}
}

func TestTemplateIsAValidSubmission(t *testing.T) {
	var s Submission
	if err := yaml.Unmarshal(Template, &s); err != nil {
		t.Fatal(err)
	}
	r, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "a"), newFinding(Major, "b"), newFinding(Major, "c")}}, "h", MaxRounds)
	_ = r.Fix("R1-02", "c1")
	r.Findings[0].Status = Verified
	if _, err := Next(r, s, "h2", MaxRounds); err != nil {
		t.Errorf("the template is a valid second round: %v", err)
	}
}

func TestWaive(t *testing.T) {
	r, _ := Next(nil, Submission{Verdict: Approved, New: []Draft{newFinding(Major, "a"), newFinding(Blocker, "b"), newFinding(Minor, "c")}}, "h", MaxRounds)
	if err := r.Waive("R1-01", "accepted risk"); err != nil || r.Findings[0].Status != Waived {
		t.Errorf("a major is waived: %v", err)
	}
	for _, id := range []string{"R1-02", "R1-03", "R9-09"} {
		if err := r.Waive(id, "x"); err == nil {
			t.Errorf("%s must not be waived", id)
		}
	}
}

func TestFoldReportFindings(t *testing.T) {
	r1, _ := Next(nil, Submission{Verdict: Approved, New: []Draft{newFinding(Nit, "src/a.go:3")}}, "h1", MaxRounds)
	sec := Folded{From: "S1-01", Source: "security", Severity: Blocker, Location: "src/q.go:9", Rule: "security.parameterised-queries", Finding: "x", Required: "y"}
	if n := r1.Fold([]Folded{sec}); n != 1 || r1.Findings[1].ID != "R1-02" || r1.Findings[1].From != "S1-01" || r1.Findings[1].Status != Open {
		t.Fatalf("%d %+v", n, r1.Findings)
	}
	if n := r1.Fold([]Folded{sec}); n != 0 || len(r1.Findings) != 2 {
		t.Errorf("a report finding joins once: %d", n)
	}
	if open := r1.Blocking(); strings.Join(open, ",") != "R1-02" {
		t.Errorf("an approved round still blocks on a security blocker: %v", open)
	}
	r2, err := Next(r1, Submission{Verdict: ChangesRequested, Previous: []Resolution{{ID: "R1-01", Status: Open}, {ID: "R1-02", Status: Open}}}, "h2", MaxRounds)
	if err != nil {
		t.Fatal(err)
	}
	qa := Folded{From: "Q1-01", Source: "qa", Severity: Major, Location: "src/a.go:5", Rule: "tests.failure-paths", Finding: "x", Required: "y"}
	if n := r2.Fold([]Folded{sec, qa}); n != 1 || r2.Findings[2].ID != "R2-01" || r2.Findings[2].Source != "qa" {
		t.Errorf("carried report findings stay; a new one is numbered in this round: %d %+v", n, r2.Findings)
	}
}

func TestCheckLocation(t *testing.T) {
	lines := func(file string) (int, bool) { return 49, file == "prefs/prefs.go" }
	cases := map[string]string{
		"prefs/prefs.go": "", "prefs/prefs.go:44": "", "prefs/prefs.go:44-48": "", "prefs/prefs.go:1-49": "",
		"prefs/prefs.go:49-55": "has 49 lines", "prefs/prefs.go:0": "has 49 lines", "prefs/prefs.go:9-3": "has 49 lines",
		"prefs/prefs.go:x": "has 49 lines", "prefs/other.go:3": "no such file", ":3": "no such file",
	}
	for loc, want := range cases {
		err := CheckLocation(loc, lines)
		if (want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), want)) {
			t.Errorf("%s: %v", loc, err)
		}
	}
}

// TestVerdictFollowsTheFindings: doc 06 round rule 4 and the severity table decide the verdict,
// so a round can neither end with a blocker or major open nor go on over minors and nits alone.
func TestVerdictFollowsTheFindings(t *testing.T) {
	r1, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3"), newFinding(Minor, "src/b.go:9")}}, "h1", MaxRounds)
	if err := r1.Fix("R1-01", "c1"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		prev    *Round
		sub     Submission
		wantErr string
	}{
		{"approved with a new major", nil, Submission{Verdict: Approved, New: []Draft{newFinding(Major, "src/a.go:3")}}, "R1-01 block"},
		{"approved with a new blocker", nil, Submission{Verdict: Approved, New: []Draft{newFinding(Blocker, "src/a.go:3")}}, "R1-01 block"},
		{"changes requested with nothing", nil, Submission{Verdict: ChangesRequested}, "verdict is approved"},
		{"changes requested over a nit", nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Nit, "src/a.go:3")}}, "verdict is approved"},
		{"changes requested over an earlier open minor", r1, Submission{Verdict: ChangesRequested,
			Previous: []Resolution{{ID: "R1-01", Status: Verified}, {ID: "R1-02", Status: Open, Note: "still"}}}, "verdict is approved"},
		{"approved with an earlier major still open", r1, Submission{Verdict: Approved,
			Previous: []Resolution{{ID: "R1-01", Status: Open, Note: "not yet"}, {ID: "R1-02", Status: Open}}}, "R1-01 block"},
		{"approved with only minors open", r1, Submission{Verdict: Approved,
			Previous: []Resolution{{ID: "R1-01", Status: Verified}, {ID: "R1-02", Status: Open}}, New: []Draft{newFinding(Minor, "src/c.go:2")}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := Next(c.prev, c.sub, "h", MaxRounds)
			if c.wantErr == "" {
				if err != nil || !r.Done() {
					t.Fatalf("want done: %v", err)
				}
				return
			}
			if !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("want %q, got %v", c.wantErr, err)
			}
		})
	}
}

func TestUnresolved(t *testing.T) {
	r1, _ := Next(nil, Submission{Verdict: ChangesRequested, New: []Draft{newFinding(Major, "src/a.go:3"), newFinding(Minor, "src/b.go:9"), newFinding(Nit, "src/c.go:1")}}, "h1", MaxRounds)
	_ = r1.Fix("R1-01", "c1")
	_ = r1.Fix("R1-02", "c1")
	r2, err := Next(r1, Submission{Verdict: Approved, Previous: []Resolution{{ID: "R1-01", Status: Verified}, {ID: "R1-02", Status: Verified}, {ID: "R1-03", Status: Open}}}, "h2", MaxRounds)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range r2.Unresolved() {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "R1-03" {
		t.Errorf("unresolved = %v, want only the open nit", ids)
	}
}
