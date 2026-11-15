// Package defects is M4's reviewer eval (EC-2, M4-plan §4.10): prepared diffs on the shop
// fixture's Go repos, each with one planted defect from doc 06's review checklist and code
// quality standard, plus clean diffs. The reviewer (and the security agent for security
// defects) reviews each in a fresh context through the same manifests the build loop uses; a
// defect is caught when a blocker or major finding overlaps its planted lines and cites one of
// its rules.
package defects

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Criterion is the case's one acceptance criterion (AC-1).
type Criterion struct {
	Given string `yaml:"given"`
	When  string `yaml:"when"`
	Then  string `yaml:"then"`
}

// Planted is where the defect is: a file of the case and an inclusive 1-based line range.
type Planted struct {
	File  string `yaml:"file"`
	Lines [2]int `yaml:"lines"`
}

// Case is one prepared diff: new or replaced files in a fixture repo.
type Case struct {
	ID          string            `yaml:"id"`
	Repo        string            `yaml:"repo"`
	Kind        string            `yaml:"kind"` // defect | clean
	Category    string            `yaml:"category"`
	Rules       []string          `yaml:"rules"`
	Security    bool              `yaml:"security"`
	Requirement string            `yaml:"requirement"`
	Criterion   Criterion         `yaml:"criterion"`
	Files       map[string]string `yaml:"files"`
	Planted     *Planted          `yaml:"planted"`
}

// Paths are the task's planned paths: every folder the case's files are in.
func (c Case) Paths() []string {
	seen := map[string]bool{}
	var out []string
	for f := range c.Files {
		p := filepath.ToSlash(filepath.Dir(f)) + "/**"
		if filepath.Dir(f) == "." {
			p = f
		}
		if !seen[p] {
			seen[p], out = true, append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// FileNames are the case's files, sorted.
func (c Case) FileNames() []string {
	out := make([]string, 0, len(c.Files))
	for f := range c.Files {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Load reads cases.yaml next to this file.
func Load() ([]Case, error) {
	_, self, _, _ := runtime.Caller(0) // this file's own path is always known
	b, err := os.ReadFile(filepath.Join(filepath.Dir(self), "cases.yaml"))
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var doc struct {
		Cases []Case `yaml:"cases"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("cases.yaml: %w", err)
	}
	return doc.Cases, nil
}

// Overlaps reports whether a finding's location (path:line or path:from-to) overlaps the
// planted lines.
func (p Planted) Overlaps(location string) bool {
	file, lines, ok := strings.Cut(location, ":")
	if !ok || filepath.Clean(file) != filepath.Clean(p.File) {
		return false
	}
	from, to := 0, 0
	if a, b, ranged := strings.Cut(lines, "-"); ranged {
		if _, err := fmt.Sscanf(a+" "+b, "%d %d", &from, &to); err != nil {
			return false
		}
	} else if _, err := fmt.Sscanf(lines, "%d", &from); err != nil {
		return false
	} else {
		to = from
	}
	return from <= p.Lines[1] && to >= p.Lines[0]
}
