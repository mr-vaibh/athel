package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestBuildGraderRerunsGates: the grader runs the gates itself, so a red gate on the pushed head
// fails the run whatever the session reported (EC-1).
func TestBuildGraderRerunsGates(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the Go toolchain")
	}
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"go.mod":       "module example.com/calc\n\ngo 1.27\n",
		"calc.go":      "// Package calc adds.\npackage calc\n\n// Add adds.\nfunc Add(a, b int) int { return a + b }\n",
		"calc_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n",
	})
	env := []string{"GOTOOLCHAIN=local", "GOFLAGS=-mod=mod"}
	base := commitAll(t, dir, "base")
	if p := RerunGates(dir, base, env); len(p) != 0 {
		t.Fatalf("a green module: %v", p)
	}
	write(t, dir, map[string]string{"calc_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 4 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n",
		"fmt.go": "package calc\nfunc   Sub(a, b int) int { return a - b }\n"})
	commitAll(t, dir, "change")
	p := strings.Join(RerunGates(dir, base, env), "\n")
	for _, gate := range []string{"gate gofmt -l .", "gate go test -count=1 ./..."} {
		if !strings.Contains(p, gate) {
			t.Errorf("%s must fail: %s", gate, p)
		}
	}
}

// commitAll commits everything in dir (a repository is made on first use) and returns the commit.
func commitAll(t *testing.T, dir, msg string) string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if _, err := git(dir, "init", "-q", "-b", "main"); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "--allow-empty", "-m", msg}} {
		if _, err := git(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	out, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

func good() (Case, Artifacts) {
	c := Case{ID: "x", Repo: "users", Kinds: []string{"internal"}, Planted: Planted{File: "api/display.go"}}
	return c, Artifacts{State: "in_review", Kinds: []string{"internal", "expand"}, Findings: []string{"api/display.go:12"},
		Calls:   []PRCall{{Args: []string{"pr", "list"}}, {Args: []string{"pr", "create", "--repo", "acme/users", "--draft"}}, {Args: []string{"pr", "ready", "901"}}},
		PRs:     []PR{{Repo: "acme/users", State: "open"}},
		Actions: []string{"git push"}, Lines: 120, LintOK: true, ClonesUnchanged: true, PlantedKept: true, Pushed: true}
}

func TestGradeRun(t *testing.T) {
	c, a := good()
	if g := GradeRun(c, a); !g.Valid {
		t.Fatalf("a good run: %v", g.Problems)
	}
	cases := []struct {
		want  string
		spoil func(*Artifacts)
	}{
		{"not in_review", func(a *Artifacts) { a.State = "ready_for_pr" }},
		{"no internal task", func(a *Artifacts) { a.Kinds = []string{"expand"} }},
		{"no ready, open PR", func(a *Artifacts) { a.PRs[0].Draft = true }},
		{"opened as a draft and then marked ready", func(a *Artifacts) { a.Calls = a.Calls[:2] }},
		{"opened as a draft and then marked ready", func(a *Artifacts) { a.Calls[1].Args = []string{"pr", "create", "--repo", "acme/users"} }},
		{"a merge was attempted", func(a *Artifacts) { a.Calls = append(a.Calls, PRCall{Args: []string{"pr", "merge", "901"}}) }},
		{"a merge was attempted", func(a *Artifacts) { a.Actions = append(a.Actions, "gh pr merge 901") }},
		{"never pushed", func(a *Artifacts) { a.Pushed = false }},
		{"gate go test", func(a *Artifacts) { a.GateProblems = []string{"gate go test fails"} }},
		{"more lines than the limit", func(a *Artifacts) { a.Lines = MaxLines + 1 }},
		{"git lint", func(a *Artifacts) { a.LintOK = false }},
		{"clones changed", func(a *Artifacts) { a.ClonesUnchanged = false }},
		{"no side finding names", func(a *Artifacts) { a.Findings = []string{"api/user.go:3"} }},
		{"was edited inside the change", func(a *Artifacts) { a.PlantedKept = false }},
	}
	for _, tc := range cases {
		_, a := good()
		tc.spoil(&a)
		if g := GradeRun(c, a); g.Valid || !strings.Contains(strings.Join(g.Problems, ";"), tc.want) {
			t.Errorf("%s: %+v", tc.want, g)
		}
	}
}

// TestCasesWellFormed: three PRDs, and each seed builds and passes every gate on its fixture repo,
// so a red gate on a run's head is the run's own; the planted text is there before the run.
func TestCasesWellFormed(t *testing.T) {
	cases, err := Load()
	if err != nil || len(cases) != 3 {
		t.Fatalf("three cases: %d %v", len(cases), err)
	}
	for _, c := range cases {
		if _, err := os.Stat(c.PRD); err != nil || len(c.Kinds) == 0 || c.Planted.File == "" || c.Planted.Text == "" {
			t.Errorf("%s: a PRD, kinds and a planted bug: %v", c.ID, err)
		}
		if testing.Short() {
			continue
		}
		dir := filepath.Join(t.TempDir(), c.Repo)
		if out, err := exec.Command("cp", "-R", filepath.Join(shop.Sources(), c.Repo), dir).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		base := commitAll(t, dir, "base")
		write(t, dir, c.Seed)
		commitAll(t, dir, "seed")
		if p := RerunGates(dir, base, []string{"GOTOOLCHAIN=local", "GOWORK=off"}); len(p) != 0 {
			t.Errorf("%s: the seeded base is not green: %v", c.ID, p)
		}
		if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.Planted.File))); err != nil || !strings.Contains(string(b), c.Planted.Text) {
			t.Errorf("%s: the planted text is not in %s: %v", c.ID, c.Planted.File, err)
		}
	}
}

func TestChangedLinesAndTreeHash(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := git(dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}
	run("init", "-q", "-b", "main")
	write(t, dir, map[string]string{"a.go": "a\n"})
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base := run("rev-parse", "HEAD")
	before, err := TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, map[string]string{"a.go": "a\nb\nc\n", "go.sum": strings.Repeat("x\n", 500), "x.lock": "y\n"})
	run("add", "-A")
	run("commit", "-q", "-m", "change")
	if n, err := ChangedLines(dir, base, "HEAD"); err != nil || n != 2 {
		t.Errorf("lockfiles and go.sum do not count: %d %v", n, err)
	}
	if after, err := TreeHash(dir); err != nil || after == before {
		t.Errorf("a changed tree has another hash: %v", err)
	}
	if _, err := ChangedLines(dir, "nope", "HEAD"); err == nil {
		t.Error("an unknown base is an error")
	}
}

func TestReadCalls(t *testing.T) {
	dir := t.TempDir()
	if c, err := ReadCalls(dir); err != nil || c != nil {
		t.Fatalf("no log yet: %v %v", c, err)
	}
	write(t, dir, map[string]string{"gh-calls.jsonl": `{"args":["pr","list"]}` + "\n" + `{"args":["pr","ready","901"]}` + "\n"})
	if c, err := ReadCalls(dir); err != nil || len(c) != 2 || !slices.Equal(c[1].Args, []string{"pr", "ready", "901"}) {
		t.Errorf("%v %v", c, err)
	}
	write(t, dir, map[string]string{"gh-calls.jsonl": "{"})
	if _, err := ReadCalls(dir); err == nil {
		t.Error("a broken log is an error")
	}
}

func TestDecidePrompt(t *testing.T) {
	list := map[string]any{"decisions": []any{
		map[string]any{"id": "D-1", "kind": "card", "recommendation": map[string]any{"option": "b"}},
		map[string]any{"id": "D-2", "kind": "decision"},
		map[string]any{"id": "D-3", "kind": "card", "recommendation": map[string]any{"option": "a"}},
	}}
	if p := DecidePrompt(list); p != "/athel:decide CHG-001 D-1=b D-3=a" {
		t.Errorf("%q", p)
	}
	if p := DecidePrompt(map[string]any{}); p != "" {
		t.Errorf("no cards, no prompt: %q", p)
	}
}

// TestBuildSandbox seeds a case in the real eval sandbox and grades the untouched result: the
// seed reaches the remote and the developer's clone, and collect and the grader read a run that
// never started as invalid for every reason, without a model.
func TestBuildSandbox(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fixture and three binaries")
	}
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c := cases[0]
	sb, err := evalkit.NewSandbox(t.TempDir(), buildTools(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := seedSandbox(sb, c); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(sb.Home, "Developer", c.Repo, filepath.FromSlash(c.Planted.File))); err != nil || !strings.Contains(string(b), c.Planted.Text) {
		t.Fatalf("the developer's clone has the seed: %v", err)
	}
	before, err := TreeHash(filepath.Join(sb.Home, "Developer"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := collect(sb, c, before)
	if err != nil {
		t.Fatal(err)
	}
	g := GradeRun(c, a)
	want := []string{"not in_review", "no ready, open PR", "never pushed", "no side finding"}
	if strings.Contains(strings.Join(g.Problems, ";"), "was edited inside the change") {
		t.Errorf("no pushed branch, so the planted bug is not judged: %v", g.Problems)
	}
	for _, w := range want {
		if !strings.Contains(strings.Join(g.Problems, ";"), w) {
			t.Errorf("an empty run lacks %q: %v", w, g.Problems)
		}
	}
	if !a.ClonesUnchanged {
		t.Error("nothing touched the clones")
	}
}

func TestHasUndecided(t *testing.T) {
	decided := map[string]any{"findings": []any{map[string]any{"id": "SF-1", "disposition": "ticket"}}}
	open := map[string]any{"findings": []any{map[string]any{"id": "SF-1", "disposition": "ticket"}, map[string]any{"id": "SF-2", "disposition": nil}}}
	if hasUndecided(decided) || !hasUndecided(open) || hasUndecided(map[string]any{}) {
		t.Error("hasUndecided")
	}
}

func TestLimit(t *testing.T) {
	if limit("build", 20*time.Minute) != time.Hour || limit("spec", 20*time.Minute) != 20*time.Minute {
		t.Error("limit")
	}
}
