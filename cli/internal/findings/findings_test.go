package findings

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func draft(cat, sev, effort, risk string) Draft {
	return Draft{FoundBy: "reviewer", FoundIn: Where{Step: "build", Task: "T-03"}, Location: "users/src/a.go:88",
		Category: cat, Severity: sev, Effort: effort, FixRisk: risk, Evidence: "seen", SuggestedFix: "do it"}
}

func TestAddGivesIDs(t *testing.T) {
	var f File
	for _, d := range []Draft{draft("bug", "medium", "S", "low"), draft("docs", "low", "M", "low")} {
		if _, err := f.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	if f.Findings[0].ID != "SF-1" || f.Findings[1].ID != "SF-2" || f.Findings[0].Disposition != nil {
		t.Errorf("%+v", f.Findings)
	}
	bad := []Draft{
		draft("vibes", "low", "S", "low"), draft("bug", "urgent", "S", "low"), draft("bug", "low", "XL", "low"),
		draft("bug", "low", "S", "none"), {Category: "bug", Severity: "low", Effort: "S", FixRisk: "low"},
	}
	for i, d := range bad {
		if _, err := f.Add(d); !aerr.HasCode(err, aerr.StateInvalid) {
			t.Errorf("draft %d: %v", i, err)
		}
	}
}

// TestRanked (doc 07): security first, then severity high to low, then small effort and low fix
// risk first; effort S with fix risk low is a quick win.
func TestRanked(t *testing.T) {
	var f File
	for _, d := range []Draft{
		draft("bug", "medium", "M", "low"),    // SF-1
		draft("security", "low", "L", "high"), // SF-2
		draft("bug", "high", "S", "medium"),   // SF-3
		draft("bug", "medium", "S", "low"),    // SF-4 quick win
		draft("dead-code", "low", "S", "low"), // SF-5 quick win
		draft("security", "high", "M", "low"), // SF-6
	} {
		if _, err := f.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, x := range f.Ranked() {
		label := x.ID
		if x.QuickWin {
			label += "*"
		}
		got = append(got, label)
	}
	if strings.Join(got, ",") != "SF-6,SF-2,SF-3,SF-4*,SF-1,SF-5*" {
		t.Errorf("%v", got)
	}
}

func TestTriage(t *testing.T) {
	var f File
	for range 3 {
		if _, err := f.Add(draft("bug", "low", "S", "low")); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Triage(map[string]string{"SF-1": FixPR, "SF-2": Ticket}, "human", "2026-11-15T10:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	if *f.Findings[0].Disposition != FixPR || f.Findings[0].DecidedBy != "human" || f.Findings[2].Disposition != nil {
		t.Errorf("%+v", f.Findings)
	}
	if len(f.Undecided()) != 1 {
		t.Errorf("undecided: %v", f.Undecided())
	}
	for _, bad := range []map[string]string{{"SF-9": Ticket}, {"SF-3": "later"}, {}} {
		if err := f.Triage(bad, "human", "x"); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}

func TestPresentation(t *testing.T) {
	var f File
	for _, d := range []Draft{draft("bug", "medium", "S", "low"), draft("security", "low", "L", "high"), draft("bug", "high", "M", "low")} {
		if _, err := f.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.Summary(); got != "Found 3 issues outside this change. 1 quick win: SF-1. Fix them as separate PRs, file tickets, or ignore?" {
		t.Errorf("%q", got)
	}
	if got := (&File{}).Summary(); got != "No issues found outside this change." {
		t.Errorf("%q", got)
	}
}
