// Package gates is doc 06's quality gates: commands with pass criteria, run in a task's worktree,
// judged deterministically against the baseline taken before the task's first edit
// (FR-BUILD-009). This package holds the defaults and the judging; running the commands, through
// the policy engine (ADR 023), is the CLI's.
package gates

import (
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Languages with default gates.
const (
	Go   = "go"
	Node = "node"
)

// Stages: cheap gates run first, so the expensive ones never run on broken code (doc 06).
const (
	Cheap = "cheap"
	Full  = "full"
)

// Pass criteria.
const (
	ExitZero    = "exit-zero"    // the command exits 0
	NoOutput    = "no-output"    // exits 0 and prints nothing (a formatter's list of files to fix)
	NewFindings = "new-findings" // no finding the baseline did not have, and no new disable comment
	Tests       = "tests"        // no failure, skip or deletion compared with the baseline
	Coverage    = "coverage"     // changed lines ≥ 90% covered; the total drops ≤ 0.5 points
	Complexity  = "complexity"   // no function newly over complexity 15 or 80 lines
)

// ProfilePlaceholder in a coverage command is replaced by the profile file the CLI reads.
const ProfilePlaceholder = "{profile}"

// Who may waive a gate (doc 06 table; waivers come in M4 task 09).
const (
	Never         = "never"
	Human         = "human"
	SecurityOwner = "security-owner"
)

// Gate is one gate: the command (argv, never a shell line) and how its result is judged. A
// gate with no command and no builtin check is unavailable: its tool is missing, so it blocks
// unless waived.
type Gate struct {
	ID       string   `yaml:"id" json:"id"`
	Stage    string   `yaml:"stage" json:"stage"`
	Pass     string   `yaml:"pass" json:"pass"`
	Waivable string   `yaml:"waivable" json:"waivable"`
	Command  []string `yaml:"command" json:"command"`
	Builtin  bool     `yaml:"builtin" json:"builtin"` // computed by the CLI itself, with no command
}

// Available reports whether the gate can run at all.
func (g Gate) Available() bool { return g.Builtin || len(g.Command) > 0 }

// defaultGates are doc 06's checks, in its order: format and lint, build and types, unit
// tests, then the full stage (coverage, complexity; more in M4 tasks 06–08).
var defaultGates = []Gate{
	{ID: "format", Stage: Cheap, Pass: NoOutput, Waivable: Never},
	{ID: "lint", Stage: Cheap, Pass: NewFindings, Waivable: Human},
	{ID: "build", Stage: Cheap, Pass: ExitZero, Waivable: Never},
	{ID: "types", Stage: Cheap, Pass: ExitZero, Waivable: Never},
	{ID: "unit", Stage: Cheap, Pass: Tests, Waivable: Never},
	{ID: "coverage", Stage: Full, Pass: Coverage, Waivable: Human},
	{ID: "complexity", Stage: Full, Pass: Complexity, Waivable: Human},
}

var goCommands = map[string][]string{
	"format":   {"gofmt", "-l", "."},
	"lint":     {"golangci-lint", "run", "./..."},
	"build":    {"go", "build", "./..."},
	"types":    {"go", "vet", "./..."},
	"unit":     {"go", "test", "-json", "-count=1", "./..."},
	"coverage": {"go", "test", "-count=1", "-coverprofile=" + ProfilePlaceholder, "./..."},
}

// nodeScripts are the package.json scripts a Node repo's gates run.
var nodeScripts = map[string]string{"format": "format:check", "lint": "lint", "build": "build", "types": "typecheck", "unit": "test"}

// Defaults are the gates of a repo in lang. A Node gate runs its package.json script and is
// unavailable without one; a repo in a language Athel has no defaults for has none available.
func Defaults(lang string, scripts map[string]bool) []Gate {
	out := make([]Gate, 0, len(defaultGates))
	for _, g := range defaultGates {
		switch lang {
		case Go:
			g.Command = goCommands[g.ID]
			g.Builtin = g.Pass == Complexity
		case Node:
			if s := nodeScripts[g.ID]; scripts[s] {
				g.Command = []string{"npm", "run", "--silent", s}
			}
		}
		out = append(out, g)
	}
	return out
}

// Detect is a repo's language from its top-level files.
func Detect(files []string) string {
	switch {
	case slices.Contains(files, "go.mod"):
		return Go
	case slices.Contains(files, "package.json"):
		return Node
	}
	return ""
}

// finding is a linter line "path:line[:col]: message"; the position is dropped so a finding
// that only moved is not new.
var finding = regexp.MustCompile(`^(\S+?):\d+(?::\d+)?: (.+)$`)

// NewLines lists the findings in now that base does not have as often.
func NewLines(base, now string) []string {
	seen := map[string]int{}
	for _, l := range strings.Split(base, "\n") {
		if m := finding.FindStringSubmatch(l); m != nil {
			seen[m[1]+": "+m[2]]++
		}
	}
	var out []string
	for _, l := range strings.Split(now, "\n") {
		m := finding.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		key := m[1] + ": " + m[2]
		if seen[key] > 0 {
			seen[key]--
			continue
		}
		out = append(out, l)
	}
	return out
}

// disable matches comments that switch a linter off (doc 06: "no new disable comments").
var disable = regexp.MustCompile(`//\s*nolint\b|eslint-disable|#\s*noqa\b|@ts-ignore|@ts-nocheck|pylint:\s*disable`)

// DisableComments lists the added lines that switch a linter off.
func DisableComments(added string) []string {
	var out []string
	for _, l := range strings.Split(added, "\n") {
		if disable.MatchString(l) {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// Test results.
const (
	Pass = "pass"
	Fail = "fail"
	Skip = "skip"
)

// TestRun is a test run: each test's result by "package Test", and the packages that failed
// with no failing test (they did not build).
type TestRun struct {
	Tests          map[string]string `yaml:"tests" json:"tests"`
	BrokenPackages []string          `yaml:"broken_packages" json:"broken_packages"`
}

// ParseGoTest reads `go test -json`.
func ParseGoTest(out string) TestRun {
	r := TestRun{Tests: map[string]string{}}
	failedPkgs := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		var e struct{ Action, Package, Test string }
		if json.Unmarshal([]byte(line), &e) != nil || (e.Action != Pass && e.Action != Fail && e.Action != Skip) {
			continue
		}
		if e.Test == "" {
			failedPkgs[e.Package] = failedPkgs[e.Package] || e.Action == Fail
			continue
		}
		r.Tests[e.Package+" "+e.Test] = e.Action
	}
	for pkg, failed := range failedPkgs {
		if failed && !slices.ContainsFunc(keys(r.Tests), func(k string) bool { return strings.HasPrefix(k, pkg+" ") && r.Tests[k] == Fail }) {
			r.BrokenPackages = append(r.BrokenPackages, pkg)
		}
	}
	sort.Strings(r.BrokenPackages)
	return r
}

// TestVerdict compares a run with the baseline (doc 06 edge cases): tests failing on base
// never fail the task; regressions, new failures, deletions, skips and packages that no longer
// build do.
type TestVerdict struct {
	Regressed   []string `json:"regressed"`
	NewFailing  []string `json:"new_failing"`
	Deleted     []string `json:"deleted"`
	Skipped     []string `json:"skipped"`
	Broken      []string `json:"broken"`
	BaseFailing []string `json:"base_failing"`
}

// Green reports a passing unit gate.
func (v TestVerdict) Green() bool {
	return len(v.Regressed)+len(v.NewFailing)+len(v.Deleted)+len(v.Skipped)+len(v.Broken) == 0
}

// CompareTests judges now against base.
func CompareTests(base, now TestRun) TestVerdict {
	var v TestVerdict
	for _, id := range keys(now.Tests) {
		was, existed := base.Tests[id]
		switch {
		case now.Tests[id] == Fail && was == Fail:
			v.BaseFailing = append(v.BaseFailing, id)
		case now.Tests[id] == Fail && existed:
			v.Regressed = append(v.Regressed, id)
		case now.Tests[id] == Fail:
			v.NewFailing = append(v.NewFailing, id)
		case now.Tests[id] == Skip && was == Pass:
			v.Skipped = append(v.Skipped, id)
		}
	}
	for _, id := range keys(base.Tests) {
		if _, ok := now.Tests[id]; !ok && base.Tests[id] != Skip {
			v.Deleted = append(v.Deleted, id)
		}
	}
	v.Broken = append(v.Broken, now.BrokenPackages...)
	return v
}

// SplitTestID splits "package Test" into its parts.
func SplitTestID(id string) (string, string) {
	pkg, name, _ := strings.Cut(id, " ")
	return pkg, name
}

// RunPattern is the -run pattern that selects exactly one test or subtest.
func RunPattern(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	return strings.Join(parts, "/")
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
