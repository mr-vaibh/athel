package defects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefectGrader(t *testing.T) {
	d := Case{ID: "D01", Kind: "defect", Category: "errors", Rules: []string{"errors.no-silent-catch"},
		Planted: &Planted{File: "carts/store.go", Lines: [2]int{20, 24}}}
	clean := Case{ID: "C01", Kind: "clean"}
	hit := Finding{ID: "R1-02", Severity: "major", Location: "carts/store.go:22", Rule: "errors.no-silent-catch"}
	cases := []struct {
		name   string
		c      Case
		fs     []Finding
		saved  bool
		caught bool
		alarms int
	}{
		{"caught", d, []Finding{{ID: "R1-01", Severity: "nit", Location: "carts/store.go:22", Rule: "errors.no-silent-catch"}, hit}, true, true, 0},
		{"minor is not enough", d, []Finding{{Severity: "minor", Location: "carts/store.go:22", Rule: "errors.no-silent-catch"}}, true, false, 0},
		{"wrong place", d, []Finding{{Severity: "major", Location: "carts/store.go:40", Rule: "errors.no-silent-catch"}}, true, false, 0},
		{"wrong rule", d, []Finding{{Severity: "blocker", Location: "carts/store.go:21-23", Rule: "readability.precise-names"}}, true, false, 0},
		{"other file", d, []Finding{{Severity: "major", Location: "carts/handler.go:22", Rule: "errors.no-silent-catch"}}, true, false, 0},
		{"no round", d, nil, false, false, 0},
		{"clean, quiet", clean, []Finding{{Severity: "nit"}, {Severity: "minor"}}, true, false, 0},
		{"clean, false alarms", clean, []Finding{{Severity: "major"}, {Severity: "blocker"}, {Severity: "minor"}}, true, false, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := GradeRun(tc.c, tc.fs, tc.saved)
			if g.Caught != tc.caught || g.FalseAlarms != tc.alarms || (len(g.Problems) > 0) == tc.saved {
				t.Errorf("%+v", g)
			}
		})
	}
	if g := GradeRun(d, []Finding{hit}, true); g.MatchedBy != "R1-02" {
		t.Errorf("matched by %q", g.MatchedBy)
	}
}

func TestSummarise(t *testing.T) {
	cases := []Case{{ID: "D01", Kind: "defect", Category: "errors"}, {ID: "D02", Kind: "defect", Category: "security"}, {ID: "C01", Kind: "clean"}}
	grades := map[string][]Grade{
		"D01": {{Caught: true}, {Caught: true}},
		"D02": {{Caught: true, Problems: []string{"no review round was saved"}}, {Caught: false}},
		"C01": {{FalseAlarms: 1}, {}},
	}
	s := Summarise(cases, grades)
	if s.DefectRuns != 4 || s.Caught != 2 || s.CatchRate != 0.5 || s.Met || s.CleanRuns != 2 || s.FalseAlarmRuns != 1 ||
		s.Missed["D02"] != 2 || s.ByCategory["errors"] != [2]int{2, 2} || len(s.Errors["D02"]) != 1 {
		t.Errorf("%+v", s)
	}
	if s := Summarise(cases[:1], map[string][]Grade{"D01": {{Caught: true}}}); !s.Met {
		t.Errorf("all caught meets the bar: %+v", s)
	}
}

func TestReadRound(t *testing.T) {
	ws := t.TempDir()
	if fs, saved, err := ReadRound(ws); err != nil || saved || fs != nil {
		t.Fatalf("no round yet: %v %v %v", fs, saved, err)
	}
	dir := filepath.Join(ws, ".athel", "changes", Change, "reviews", Task)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for n, body := range map[string]string{"1": "findings: [{id: R1-01, severity: major}]\n", "2": "findings: [{id: R1-01, severity: major}, {id: R2-01, severity: nit, source: security}]\n"} {
		if err := os.WriteFile(filepath.Join(dir, "round-"+n+".yaml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fs, saved, err := ReadRound(ws)
	if err != nil || !saved || len(fs) != 2 || fs[1].Source != "security" {
		t.Errorf("the latest round: %+v %v %v", fs, saved, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "round-2.yaml"), []byte("findings: {"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadRound(ws); err == nil {
		t.Error("a broken round is an error")
	}
}
