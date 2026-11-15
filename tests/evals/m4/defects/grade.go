package defects

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// Finding is a review round's finding, as the grader reads it.
type Finding struct {
	ID       string `yaml:"id" json:"id"`
	Severity string `yaml:"severity" json:"severity"`
	Source   string `yaml:"source" json:"source"`
	Location string `yaml:"location" json:"location"`
	Rule     string `yaml:"rule" json:"rule"`
}

// Grade is one run's result.
type Grade struct {
	Caught      bool     `json:"caught"`
	MatchedBy   string   `json:"matched_by,omitempty"`
	FalseAlarms int      `json:"false_alarms"` // blocker or major findings on a clean diff
	Findings    int      `json:"findings"`
	Problems    []string `json:"problems,omitempty"`
}

func blocking(f Finding) bool { return f.Severity == "blocker" || f.Severity == "major" }

// GradeRun grades a run's findings: a defect is caught by a blocker or major finding whose
// location overlaps the planted lines and whose rule is one of the defect's (M4-plan EC-2).
func GradeRun(c Case, fs []Finding, roundSaved bool) Grade {
	g := Grade{Findings: len(fs)}
	if !roundSaved {
		g.Problems = append(g.Problems, "no review round was saved")
	}
	for _, f := range fs {
		switch {
		case c.Kind == "clean" && blocking(f):
			g.FalseAlarms++
		case c.Kind == "defect" && !g.Caught && blocking(f) && c.Planted.Overlaps(f.Location) && slices.Contains(c.Rules, f.Rule):
			g.Caught, g.MatchedBy = true, f.ID
		}
	}
	return g
}

// ReadRound reads the task's latest review round from the workspace: its findings include the
// security report's, which join the round by themselves.
func ReadRound(ws string) ([]Finding, bool, error) {
	dir := filepath.Join(ws, ".athel", "changes", Change, "reviews", Task)
	for n := 5; n >= 1; n-- {
		b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("round-%d.yaml", n))) //nolint:gosec // a round file of the run's own sandbox
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, false, err
		}
		var r struct {
			Findings []Finding `yaml:"findings"`
		}
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, false, err
		}
		return r.Findings, true, nil
	}
	return nil, false, nil
}

// Summary is EC-2 over every run.
type Summary struct {
	DefectRuns     int                 `json:"defect_runs"`
	Caught         int                 `json:"caught"`
	CatchRate      float64             `json:"catch_rate"`
	CleanRuns      int                 `json:"clean_runs"`
	FalseAlarmRuns int                 `json:"false_alarm_runs"`
	ByCategory     map[string][2]int   `json:"by_category"` // caught, runs
	Missed         map[string]int      `json:"missed"`      // case ID → runs that missed it
	Met            bool                `json:"met"`
	Errors         map[string][]string `json:"errors,omitempty"`
}

// Bar is EC-2's catch rate.
const Bar = 0.9

// Summarise adds up the graded runs; a run with a problem counts as not caught.
func Summarise(cases []Case, grades map[string][]Grade) Summary {
	s := Summary{ByCategory: map[string][2]int{}, Missed: map[string]int{}, Errors: map[string][]string{}}
	for _, c := range cases {
		for _, g := range grades[c.ID] {
			s.Errors[c.ID] = append(s.Errors[c.ID], g.Problems...)
			if c.Kind == "clean" {
				s.CleanRuns++
				if g.FalseAlarms > 0 {
					s.FalseAlarmRuns++
				}
				continue
			}
			s.DefectRuns++
			cat := s.ByCategory[c.Category]
			cat[1]++
			if g.Caught && len(g.Problems) == 0 {
				s.Caught++
				cat[0]++
			} else {
				s.Missed[c.ID]++
			}
			s.ByCategory[c.Category] = cat
		}
		if len(s.Errors[c.ID]) == 0 {
			delete(s.Errors, c.ID)
		}
	}
	if s.DefectRuns > 0 {
		s.CatchRate = float64(s.Caught) / float64(s.DefectRuns)
	}
	s.Met = s.DefectRuns > 0 && s.CatchRate >= Bar
	return s
}
