// Package m3 holds M3's evaluation: eval PRDs on the shop fixture, their expected outcomes, and
// (M3 task 13) the grader and runner.
package m3

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Expect is a case's expected outcome.
type Expect struct {
	Affected      map[string][][]string `yaml:"affected"`
	Risk          string                `yaml:"risk"`
	RiskAtLeast   string                `yaml:"risk_at_least"`
	CardRules     []string              `yaml:"card_rules"`
	ReviewMissing []string              `yaml:"review_missing"`
	Ordering      []string              `yaml:"ordering"`
}

// Case is one eval PRD.
type Case struct {
	ID     string `yaml:"id"`
	PRD    string `yaml:"prd"`
	Kind   string `yaml:"kind"`
	Expect Expect `yaml:"expect"`
}

func loadCases(t *testing.T) []Case {
	t.Helper()
	b, err := os.ReadFile("cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []Case `yaml:"cases"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	return doc.Cases
}

// TestCasesAreWellFormed checks what the grader relies on: three PRDs that exist, a risk
// expectation each, sections and rules from doc 05's vocabulary.
func TestCasesAreWellFormed(t *testing.T) {
	cases := loadCases(t)
	if len(cases) < 3 {
		t.Fatalf("%d cases, the M3 exit criterion needs 3", len(cases))
	}
	levels := []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}
	sections := []string{"direct", "consumers", "data", "infra", "people", "runtime"}
	kinds := []string{"expand", "migrate", "consume", "contract", "internal"}
	for _, c := range cases {
		if _, err := os.Stat(filepath.Clean(c.PRD)); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
		if (c.Expect.Risk == "") == (c.Expect.RiskAtLeast == "") || !slices.Contains(levels, c.Expect.Risk+c.Expect.RiskAtLeast) {
			t.Errorf("%s: give exactly one of risk or risk_at_least, a doc 05 level", c.ID)
		}
		for s, items := range c.Expect.Affected {
			if !slices.Contains(sections, s) || len(items) == 0 {
				t.Errorf("%s: section %q", c.ID, s)
			}
		}
		checkVocabulary(t, c, kinds)
	}
}

// checkVocabulary: ask rules are A1–A7 and task kinds are plan@2's.
func checkVocabulary(t *testing.T, c Case, kinds []string) {
	t.Helper()
	for _, r := range c.Expect.CardRules {
		if len(r) != 2 || r[0] != 'A' || r[1] < '1' || r[1] > '7' {
			t.Errorf("%s: ask rule %q", c.ID, r)
		}
	}
	for _, k := range c.Expect.Ordering {
		if !slices.Contains(kinds, k) {
			t.Errorf("%s: task kind %q", c.ID, k)
		}
	}
}
