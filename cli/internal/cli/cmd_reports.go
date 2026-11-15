package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/reports"
	"github.com/mr-vaibh/athel/cli/internal/review"
)

// Doc 06's role-agent reports besides the review round: the developer's build result, QA's
// report and the security report. QA and security work in parallel with the reviewer; their
// findings join the task's latest review round, whichever is saved first, and are then fixed and
// verified like the reviewer's.

func cmdBuildTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "build template", string(reports.BuildTemplate))
}

func cmdQATemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "qa template", string(reports.QATemplate))
}

func cmdSecurityTemplate(r *runCtx, args []string) (any, error) {
	return printTemplate(r, args, "security template", string(reports.SecurityTemplate))
}

// reportInput reads CHG-NNN T-NN - and the draft for a report command.
func (r *runCtx) reportInput(args []string, name string, draft any) (gateTask, string, error) {
	if len(args) != 3 {
		return gateTask{}, "", usagef("usage: athel %s CHG-NNN T-NN -", name)
	}
	data, err := r.readBlock(args[2])
	if err != nil {
		return gateTask{}, "", err
	}
	g, err := r.gateTask(args[:2], name)
	if err != nil {
		return gateTask{}, "", err
	}
	kind := strings.Fields(name)[0]
	if err := decodeStrict(data, draft, kind); err != nil {
		return gateTask{}, "", draftError(err, kind+" template")
	}
	head, err := r.gitOut(g.dir, "rev-parse", "HEAD")
	return g, strings.TrimSpace(head), err
}

// taskChanges are the files the task changed since its baseline, committed or not, and new
// files not yet added.
func (r *runCtx) taskChanges(g gateTask) ([]string, error) {
	from := g.base
	if f, err := r.loadGates(g); err == nil && f.Baseline != nil {
		from = f.Baseline.Head
	}
	diff, err := r.gitOut(g.dir, "diff", "--name-only", from)
	if err != nil {
		return nil, err
	}
	untracked, err := r.gitOut(g.dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	out := append(strings.Fields(diff), strings.Fields(untracked)...)
	slices.Sort(out)
	return slices.Compact(out), nil
}

// lineCount counts a worktree file's lines, for finding locations.
func lineCount(g gateTask) func(string) (int, bool) {
	return func(file string) (int, bool) {
		if file == "" || filepath.IsAbs(file) || strings.Contains(file, "..") {
			return 0, false
		}
		b, err := os.ReadFile(filepath.Join(g.dir, filepath.FromSlash(file)))
		if err != nil {
			return 0, false
		}
		return strings.Count(string(b), "\n") + boolInt(len(b) > 0 && b[len(b)-1] != '\n'), true
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// checkLocations refuses a finding whose file or lines are not in the worktree.
func checkLocations(g gateTask, locs ...string) error {
	for _, l := range locs {
		if err := review.CheckLocation(l, lineCount(g)); err != nil {
			return err
		}
	}
	return nil
}

func reportLocations(fs []reports.Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Location
	}
	return out
}

// nextReport is the number of a task's next report of a kind: 1 more than the files saved.
func nextReport(g gateTask, dir, prefix string) int {
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(g.s.WS.ChangeDir(g.c.ID), dir, g.task.ID, fmt.Sprintf("%s-%d.yaml", prefix, n))); err != nil {
			return n
		}
		n++
	}
}

func (r *runCtx) saveReport(g gateTask, schemaID, name string, v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "encode "+name, err)
	}
	if err := r.registry().ValidateYAML(schemaID, b); err != nil {
		return err
	}
	_, err = g.s.SaveChangeFile(g.c.ID, g.session, name, b)
	return err
}

// cmdBuildSave saves the developer's result of a round; the CLI fills in what git knows.
func cmdBuildSave(r *runCtx, args []string) (any, error) {
	var d reports.BuildDraft
	g, head, err := r.reportInput(args, "build save", &d)
	if err != nil {
		return nil, err
	}
	if err := reports.CheckBuild(d); err != nil {
		return nil, err
	}
	changed, err := r.taskChanges(g)
	if err != nil {
		return nil, err
	}
	n := nextReport(g, "build", "result")
	res := reports.Build{Schema: reports.BuildSchema, Change: g.c.ID, Task: g.task.ID, N: n, Head: head,
		FilesChanged: changed, TestsAdded: []string{}, CommandsRun: d.CommandsRun, Notes: d.Notes}
	for _, f := range changed {
		if gates.IsTestFile(f) {
			res.TestsAdded = append(res.TestsAdded, f)
		}
	}
	return res, r.saveReport(g, reports.BuildSchema, fmt.Sprintf("build/%s/result-%d.yaml", g.task.ID, n), res)
}

// cmdQASave saves QA's report: every case is a test file the task changed.
func cmdQASave(r *runCtx, args []string) (any, error) {
	var d reports.QADraft
	g, head, err := r.reportInput(args, "qa save", &d)
	if err != nil {
		return nil, err
	}
	changed, err := r.taskChanges(g)
	if err != nil {
		return nil, err
	}
	if err := reports.CheckQA(d, gates.IsTestFile, changed); err != nil {
		return nil, err
	}
	if err := checkLocations(g, reportLocations(d.Findings)...); err != nil {
		return nil, err
	}
	n := nextReport(g, "reviews", "qa")
	rep := reports.QA{Schema: reports.QASchema, Change: g.c.ID, Task: g.task.ID, N: n, Head: head,
		Cases: d.Cases, Suite: d.Suite, Gaps: nonNil(d.Gaps), Findings: reports.Number("Q", n, d.Findings)}
	if err := r.saveReport(g, reports.QASchema, fmt.Sprintf("reviews/%s/qa-%d.yaml", g.task.ID, n), rep); err != nil {
		return nil, err
	}
	return rep, r.foldIntoLatest(g)
}

// cmdSecuritySave saves the security report.
func cmdSecuritySave(r *runCtx, args []string) (any, error) {
	var d reports.SecurityDraft
	g, head, err := r.reportInput(args, "security save", &d)
	if err != nil {
		return nil, err
	}
	if err := reports.CheckSecurity(d); err != nil {
		return nil, err
	}
	if err := checkLocations(g, reportLocations(d.Findings)...); err != nil {
		return nil, err
	}
	n := nextReport(g, "reviews", "security")
	rep := reports.Security{Schema: reports.SecuritySchema, Change: g.c.ID, Task: g.task.ID, N: n, Head: head,
		Checked: d.Checked, Findings: reports.Number("S", n, d.Findings)}
	if err := r.saveReport(g, reports.SecuritySchema, fmt.Sprintf("reviews/%s/security-%d.yaml", g.task.ID, n), rep); err != nil {
		return nil, err
	}
	return rep, r.foldIntoLatest(g)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// reportFindings are every QA and security finding of the task, in report order.
func (r *runCtx) reportFindings(g gateTask) ([]review.Folded, error) {
	var out []review.Folded
	for _, kind := range []struct{ prefix, source, schema string }{{"qa", "qa", reports.QASchema}, {"security", "security", reports.SecuritySchema}} {
		for n := 1; n < nextReport(g, "reviews", kind.prefix); n++ {
			b, err := os.ReadFile(filepath.Join(g.s.WS.ChangeDir(g.c.ID), "reviews", g.task.ID, fmt.Sprintf("%s-%d.yaml", kind.prefix, n)))
			if err != nil {
				return nil, aerr.Wrap(aerr.StateIO, "read a report", err)
			}
			if err := r.registry().ValidateYAML(kind.schema, b); err != nil {
				return nil, err
			}
			var rep struct {
				Findings []reports.Finding `yaml:"findings"`
			}
			if err := yaml.Unmarshal(b, &rep); err != nil {
				return nil, aerr.Wrap(aerr.StateSchema, "a report", err)
			}
			for _, f := range rep.Findings {
				out = append(out, review.Folded{From: f.ID, Source: kind.source, Severity: f.Severity, Location: f.Location,
					Rule: f.Rule, Finding: f.Finding, Required: f.Required})
			}
		}
	}
	return out, nil
}

// foldIntoLatest adds the report findings the latest round lacks, under the change lock. Before
// round 1 there is nothing to join: review save folds them in.
func (r *runCtx) foldIntoLatest(g gateTask) error {
	rd, err := r.latestRound(g)
	if err != nil || rd == nil {
		return err
	}
	name := fmt.Sprintf("reviews/%s/round-%d.yaml", g.task.ID, rd.Round)
	_, err = g.s.UpdateChangeFile(g.c.ID, g.session, name, func(old []byte) ([]byte, error) {
		var cur review.Round
		if err := yaml.Unmarshal(old, &cur); err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, name, err)
		}
		all, err := r.reportFindings(g)
		if err != nil {
			return nil, err
		}
		cur.Fold(all)
		b, err := yaml.Marshal(&cur)
		if err != nil {
			return nil, aerr.Wrap(aerr.StateSchema, "encode the review round", err)
		}
		return b, r.registry().ValidateYAML(review.SchemaID, b)
	})
	return err
}

// unfolded lists report findings missing from the round: a backstop, so a finding saved while a
// round was being written can never be skipped.
func (r *runCtx) unfolded(g gateTask, rd *review.Round) ([]string, error) {
	all, err := r.reportFindings(g)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range all {
		if rd == nil || !slices.ContainsFunc(rd.Findings, func(x review.Finding) bool { return x.From == f.From }) {
			out = append(out, f.From)
		}
	}
	return out, nil
}
