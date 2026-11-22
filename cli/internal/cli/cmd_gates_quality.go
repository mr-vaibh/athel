package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/gates"
)

// Coverage and complexity, doc 06's full-stage gates for Go (ADR 025).

// withProfile replaces the profile placeholder of a coverage command with a fresh file outside
// the worktree, so the profile never lands in a commit. done removes it.
func withProfile(argv []string) ([]string, string, func()) {
	out := append([]string{}, argv...)
	if !strings.Contains(strings.Join(argv, " "), gates.ProfilePlaceholder) {
		return out, "", func() {}
	}
	dir, err := os.MkdirTemp("", "athel-cover-")
	if err != nil {
		return out, "", func() {}
	}
	profile := filepath.Join(dir, "cover.out")
	for i, a := range out {
		out[i] = strings.ReplaceAll(a, gates.ProfilePlaceholder, profile)
	}
	return out, profile, func() { _ = os.RemoveAll(dir) } // a throwaway folder
}

// record keeps what a gate's run on base is judged against later.
func (b *gateBaseline) record(gate gates.Gate, g gateTask, exit int, out, profile string) {
	b.Exits[gate.ID] = exit
	switch gate.Pass {
	case gates.NewFindings:
		b.Lint = out
	case gates.Tests:
		if g.lang == gates.Go {
			b.Tests = gates.ParseGoTest(out)
		}
	case gates.Coverage:
		if p, err := readProfile(g.dir, profile); err == nil {
			total := p.Total()
			b.Coverage = &total
		}
	}
}

func readProfile(dir, file string) (gates.Profile, error) {
	text, err := os.ReadFile(file) //nolint:gosec // the profile file withProfile made
	if err != nil {
		return gates.Profile{}, err
	}
	return gates.ParseProfile(string(text), goModule(dir))
}

// goModule is the module path in dir's go.mod, or "".
func goModule(dir string) string {
	f, err := os.Open(filepath.Join(dir, "go.mod")) //nolint:gosec // the worktree's own go.mod
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }() // read-only
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.TrimSpace(mod)
		}
	}
	return ""
}

// judgeCoverage: changed lines at least 90% covered, and the total no more than 0.5 points
// below the baseline's.
func (r *runCtx) judgeCoverage(g gateTask, base *gateBaseline, exit int, out, profile string, o *gateOutcome) {
	p, err := readProfile(g.dir, profile)
	if exit != 0 || err != nil {
		o.Status, o.Summary, o.Details = gates.Fail, "the tests did not run to a coverage profile", tail(out)
		return
	}
	diff, err := r.gitOut(g.dir, "diff", "-U0", "--no-color", "--no-ext-diff", base.Head)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	covered, uncovered := p.ChangedCoverage(gates.ChangedLines(diff))
	if n := covered + uncovered; n > 0 {
		if pct := 100 * float64(covered) / float64(n); pct < gates.MinChangedCoverage {
			o.Status = gates.Fail
			o.Details = append(o.Details, fmt.Sprintf("changed lines %.1f%% covered (%d of %d); at least %.0f%% needed", pct, covered, n, gates.MinChangedCoverage))
		}
	}
	if base.Coverage != nil && p.Total() < *base.Coverage-gates.MaxTotalDrop {
		o.Status = gates.Fail
		o.Details = append(o.Details, fmt.Sprintf("total coverage %.1f%%, down from %.1f%% on base (at most %.1f points)", p.Total(), *base.Coverage, gates.MaxTotalDrop))
	}
	if o.Status == gates.Fail {
		o.Summary = "coverage is below doc 06's bar"
	}
}

// judgeComplexity: no function in a changed Go file newly over the limits.
func (r *runCtx) judgeComplexity(g gateTask, base *gateBaseline, o *gateOutcome) {
	names, err := r.gitOut(g.dir, "diff", "--name-only", "--diff-filter=AM", base.Head)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	for _, name := range strings.Fields(names) {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		now, err := os.ReadFile(filepath.Join(g.dir, name)) //nolint:gosec // a changed file of the worktree
		if err != nil {
			o.Status, o.Summary = "unavailable", err.Error()
			return
		}
		was, err := r.gitOut(g.dir, "show", base.Head+":"+name)
		if err != nil {
			was = "" // a new file
		}
		found, err := gates.TooComplex(name, was, string(now), gates.DefaultLimits)
		if err != nil {
			o.Status, o.Summary, o.Details = gates.Fail, name+" does not parse", []string{err.Error()}
			return
		}
		o.Details = append(o.Details, found...)
	}
	if len(o.Details) > 0 {
		o.Status, o.Summary = gates.Fail, "functions newly over doc 06's complexity or length limits"
	}
}
