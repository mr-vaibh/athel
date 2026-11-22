package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/blast"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
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

// judgeBuiltin runs a gate Athel computes itself.
func (r *runCtx) judgeBuiltin(g gateTask, base *gateBaseline, gate gates.Gate, o *gateOutcome) {
	switch gate.Pass {
	case gates.Complexity:
		r.judgeComplexity(g, base, o)
	case gates.ACTests:
		r.judgeACTests(g, o)
	case gates.Contract:
		r.judgeContract(g, o)
	case gates.Mutation:
		r.judgeMutation(g, base, o)
	}
}

// judgeMutation runs the changed lines' mutants of a HIGH or CRITICAL change, each against its
// package's tests through a Go build overlay, so the worktree is never touched (ADR 025).
func (r *runCtx) judgeMutation(g gateTask, base *gateBaseline, o *gateOutcome) {
	risk, err := r.blastRisk(g.s, g.c.ID)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	if risk != "HIGH" && risk != "CRITICAL" {
		o.Status, o.Summary = gates.Skipped, "not required: the change's risk is "+cmpOr(risk, "not assessed")+"; mutation runs at HIGH and CRITICAL"
		return
	}
	mutants, err := r.changedMutants(g, base)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	killed, survived := 0, 0
	for _, m := range gates.Cap(mutants) {
		switch r.runMutant(g, m) {
		case gates.Fail:
			killed++
		case gates.Pass:
			survived++
			o.Details = append(o.Details, "survived: "+m.File+" "+m.Desc)
		}
	}
	if score := gates.Score(killed, survived); score < gates.MinMutationScore {
		o.Status = gates.Fail
		o.Summary = fmt.Sprintf("mutation score %.0f%% (%d of %d killed); at least %.0f%% needed", score, killed, killed+survived, gates.MinMutationScore)
	}
}

func cmpOr(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// changedMutants lists the mutants of the task's changed Go lines, file by file in name order.
func (r *runCtx) changedMutants(g gateTask, base *gateBaseline) ([]gates.Mutant, error) {
	diff, err := r.gitOut(g.dir, "diff", "-U0", "--no-color", "--no-ext-diff", base.Head)
	if err != nil {
		return nil, err
	}
	changed := gates.ChangedLines(diff)
	files := make([]string, 0, len(changed))
	for f := range changed {
		files = append(files, f)
	}
	sort.Strings(files)
	var out []gates.Mutant
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(g.dir, f)) //nolint:gosec // a changed file of the worktree
		if err != nil {
			return nil, err
		}
		ms, err := gates.Mutants(f, string(src), changed[f])
		if err != nil {
			return nil, err
		}
		out = append(out, ms...)
	}
	return out, nil
}

// runMutant runs one mutant's package tests: fail means the tests killed it, pass that it
// survived, and anything else (it does not compile, it cannot run) that it does not count.
func (r *runCtx) runMutant(g gateTask, m gates.Mutant) string {
	tmp, err := os.MkdirTemp("", "athel-mutant-")
	if err != nil {
		return ""
	}
	defer func() { _ = os.RemoveAll(tmp) }() // a throwaway folder
	mutated, overlay := filepath.Join(tmp, filepath.Base(m.File)), filepath.Join(tmp, "overlay.json")
	// Go names files by their real path, so a symlinked worktree folder (macOS /var) must be
	// resolved, or the overlay would never apply and every mutant would seem to survive.
	real, err := filepath.EvalSymlinks(filepath.Join(g.dir, m.File))
	if err != nil {
		return ""
	}
	spec, _ := json.Marshal(map[string]map[string]string{"Replace": {real: mutated}}) // strings always encode
	if os.WriteFile(mutated, []byte(m.Src), 0o600) != nil || os.WriteFile(overlay, spec, 0o600) != nil {
		return ""
	}
	argv := []string{"go", "test", "-count=1", "-json", "-overlay=" + overlay, "./" + filepath.Dir(m.File)}
	if r.gateRefusal(g.dir, argv) != "" {
		return ""
	}
	_, out, _, err := r.runGateCommand(g.dir, argv)
	if err != nil {
		return ""
	}
	run := gates.ParseGoTest(out)
	switch {
	case len(run.BrokenPackages) > 0:
		return ""
	case slices.Contains(mapValues(run.Tests), gates.Fail):
		return gates.Fail
	}
	return gates.Pass
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// judgeACTests: every AC the task covers is named by a test file changed since the change's
// branch was cut (FR-BUILD-003), so a test written by an earlier task of the change counts.
func (r *runCtx) judgeACTests(g gateTask, o *gateOutcome) {
	names, err := r.gitOut(g.dir, "diff", "--name-only", "--diff-filter=AM", g.base)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	tests := map[string]string{}
	for _, name := range strings.Fields(names) {
		if !gates.IsTestFile(name) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(g.dir, name)) //nolint:gosec // a changed file of the worktree
		if err != nil {
			o.Status, o.Summary = "unavailable", err.Error()
			return
		}
		tests[name] = string(b)
	}
	if missing := gates.MissingACs(g.task.Covers, tests); len(missing) > 0 {
		o.Status, o.Summary = gates.Fail, "no changed test names "+strings.Join(missing, ", ")
		o.Details = append(o.Details, "put the AC ID in the test's name or a comment, in the repo's test style (FR-BUILD-003)")
	}
}

// judgeContract: the API and event changes of the change's diff in this repo are all in the
// planned blast radius (doc 06 contract gate; FR-BLAST-005). With no planned report nothing was
// planned, so any contract change fails.
func (r *runCtx) judgeContract(g gateTask, o *gateOutcome) {
	diff, err := r.gitOut(g.dir, "diff", "--unified=0", "--no-color", "--no-ext-diff", g.base)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	planned, err := r.plannedReport(g.s, g.c.ID)
	if aerr.HasCode(err, aerr.StateNotFound) {
		planned, err = &blast.Report{}, nil
	}
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	if drift := blast.Drift(blast.ParseDiff(diff), planned); len(drift) > 0 {
		o.Status, o.Summary, o.Details = gates.Fail, "the diff changes contracts the plan does not name", drift
	}
}
