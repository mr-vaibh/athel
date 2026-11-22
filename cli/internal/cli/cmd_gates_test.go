package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// goGates is a building change whose users worktree is a small Go module with a passing test,
// committed, with a scripted golangci-lint on PATH: one old finding always, and a new one when
// src/bad.go exists.
func goGates(t *testing.T) (*harness, string) {
	t.Helper()
	h, tree, _, _ := building(t)
	bin := t.TempDir()
	lint := "#!/bin/sh\necho 'main.go:1:1: old finding (gosec)'\nif [ -f src/bad.go ]; then echo 'src/bad.go:3:1: new finding (revive)'; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "golangci-lint"), []byte(lint), 0o700); err != nil {
		t.Fatal(err)
	}
	cache, err := exec.Command("go", "env", "GOCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	h.env["GOCACHE"], h.env["GOFLAGS"], h.env["GOTOOLCHAIN"] = strings.TrimSpace(string(cache)), "-mod=mod", "local"
	for file, body := range map[string]string{
		"go.mod":        "module example.com/users\n\ngo 1.27\n",
		"main.go":       "package main\n\nfunc main() {}\n",
		"src/a_test.go": "package src\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
	} {
		if out, err := commit(t, h, tree, file, body, goodMsg); err != nil {
			t.Fatalf("%s: %v %s", file, err, out)
		}
	}
	return h, tree
}

func TestGateBaselineAndRun(t *testing.T) {
	h, _ := goGates(t)
	b := h.mustOK("gate", "baseline", "CHG-001", "T-01")["baseline"].(map[string]any)
	if b["tests"].(map[string]any)["tests"].(map[string]any)["example.com/users/src TestA"] != "pass" || !strings.Contains(b["lint"].(string), "old finding") {
		t.Fatalf("baseline = %v", b)
	}
	if again := h.mustOK("gate", "baseline", "CHG-001", "T-01")["baseline"].(map[string]any); again["at"] != b["at"] {
		t.Error("the baseline is taken once")
	}
	run := h.mustOK("gate", "run", "CHG-001", "T-01")
	if run["green"] != true || len(run["results"].([]any)) != 5 {
		t.Fatalf("a clean task is green, with the old lint finding not counted: %v", run)
	}
}

func TestGateRunRed(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	bad := "package src\nfunc  X(){ //nolint:all\n}\n"
	if out, err := commit(t, h, tree, "src/bad.go", bad, goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := commit(t, h, tree, "src/b_test.go", "package src\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) { t.Fatal(\"no\") }\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	code, env := h.runJSON("gate", "run", "CHG-001", "T-01")
	msg := asJSON(t, env)
	if code != aerr.ExitGate || errCode(env) != string(aerr.GateRed) {
		t.Fatalf("red: %d %v", code, env)
	}
	for _, want := range []string{"format fail", "lint fail: 2 new lint findings or disable comments", "unit fail"} {
		if !strings.Contains(msg, want) {
			t.Errorf("red gates lack %q: %s", want, msg)
		}
	}
	show := h.mustOK("gate", "show", "CHG-001", "T-01")
	runs := show["runs"].([]any)
	last := runs[len(runs)-1].(map[string]any)
	if len(runs) != 1 || last["green"] != false || !strings.Contains(asJSON(t, last), "new failing: example.com/users/src TestB") {
		t.Errorf("evidence: %v", last)
	}
	if _, err := os.Stat(filepath.Join(h.root, ".athel", "changes", "CHG-001", "artifacts", "gates", "T-01", "1-unit.log")); err != nil {
		t.Errorf("gate output kept as evidence: %v", err)
	}
}

func TestGateRefusals(t *testing.T) {
	h, tree := goGates(t)
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "no baseline yet") {
		t.Errorf("a run before the baseline: %v", env)
	}
	if err := os.WriteFile(filepath.Join(tree, "src", "wip.go"), []byte("package src\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("gate", "baseline", "CHG-001", "T-01"); errCode(env) != string(aerr.StateInvalid) || !strings.Contains(asJSON(t, env), "uncommitted changes") {
		t.Errorf("a baseline on an edited worktree: %v", env)
	}
	for _, args := range [][]string{{"gate", "run", "CHG-001", "T-09"}, {"gate", "run", "CHG-001"}, {"gate", "show", "CHG-001", "T-09"}, {"gate", "run", "--stage", "nightly", "CHG-001", "T-01"}} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
	early := approvedOnUsers(t)
	if _, env := early.runJSON("gate", "baseline", "CHG-001", "T-01"); errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("before building: %v", env)
	}
}

// TestGateUnavailable: a gate whose tool is missing is unavailable, which is red (doc 06).
func TestGateUnavailable(t *testing.T) {
	h, _ := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	h.env["PATH"] = filepath.Dir(lookPath(os.Getenv("PATH"), "go")) + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin"
	_, env := h.runJSON("gate", "run", "CHG-001", "T-01")
	if errCode(env) != string(aerr.GateRed) || !strings.Contains(asJSON(t, env), "lint unavailable: golangci-lint is not installed") {
		t.Errorf("%v", env)
	}
}

// flakyTest is a Go test that counts its runs in counter and fails on the runs listed.
func flakyTest(counter string, failOn ...int) string {
	var cases []string
	for _, n := range failOn {
		cases = append(cases, "n == "+itoa(n))
	}
	return "package src\n\nimport (\n\t\"os\"\n\t\"strconv\"\n\t\"testing\"\n)\n\n" +
		"func TestFlaky(t *testing.T) {\n\tb, _ := os.ReadFile(" + strconvQuote(counter) + ")\n\tn, _ := strconv.Atoi(string(b))\n\tn++\n" +
		"\t_ = os.WriteFile(" + strconvQuote(counter) + ", []byte(strconv.Itoa(n)), 0o600)\n\tif " + strings.Join(cases, " || ") + " {\n\t\tt.Fatal(\"flake\")\n\t}\n}\n"
}

// strconvQuote is s as a Go raw string literal.
func strconvQuote(s string) string { return "`" + s + "`" }

// TestGateFlaky (doc 06 edge case): a test that passes again on a re-run is flaky; it stops
// counting against the task only if it also flakes on the baseline.
func TestGateFlaky(t *testing.T) {
	for name, tc := range map[string]struct {
		failOn []int
		green  bool
		note   string
	}{
		// run 1 baseline, run 2 the gate, run 3 the re-run, runs 4–6 on the baseline checkout
		"flaky only after the change": {[]int{2}, false, "flaky only after this change"},
		"flaky on base too":           {[]int{2, 4}, true, "flaky on base too"},
	} {
		t.Run(name, func(t *testing.T) {
			h, tree := goGates(t)
			counter := filepath.Join(t.TempDir(), "count")
			if out, err := commit(t, h, tree, "src/flaky_test.go", flakyTest(counter, tc.failOn...), goodMsg); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			h.mustOK("gate", "baseline", "CHG-001", "T-01")
			_, env := h.runJSON("gate", "run", "CHG-001", "T-01")
			show := asJSON(t, h.mustOK("gate", "show", "CHG-001", "T-01"))
			if green := errCode(env) == ""; green != tc.green || !strings.Contains(show, tc.note) {
				t.Errorf("green %v, want %v; evidence %s", green, tc.green, show)
			}
		})
	}
}

// TestGateRefusal (ADR 023): only known, local commands that write inside the worktree run.
func TestGateRefusal(t *testing.T) {
	dir := "/ws/repos/users@CHG-001"
	for argv, want := range map[string]string{
		"go test ./...":                       "",
		"gofmt -l .":                          "",
		"curl https://example.invalid/x":      "tier R1",
		"cat .env":                            "secret",
		"xyzzy --frob":                        "does not know",
		"cp a /etc/hosts":                     "outside the worktree",
		"git push origin athel/CHG-001-x":     "push",
		"kubectl --context prod delete pod x": "tier",
	} {
		got := gateRefusal(nil, dir, strings.Fields(argv))
		if (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", argv, got, want)
		}
	}
}

func TestGateNodeDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"lint":"eslint .","test":"node --test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, g := range (gateTask{dir: dir, lang: "node"}).defaults() {
		if len(g.Command) > 0 {
			have = append(have, g.ID+"="+strings.Join(g.Command, " "))
		}
	}
	if strings.Join(have, ",") != "lint=npm run --silent lint,unit=npm run --silent test" {
		t.Errorf("%v", have)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, g := range (gateTask{dir: dir, lang: "node"}).defaults() {
		if len(g.Command) > 0 {
			t.Errorf("an unreadable package.json has no scripts: %+v", g)
		}
	}
}

func TestGateTail(t *testing.T) {
	long := strings.Repeat("line\n", 30) + "last\n"
	if got := tail(long); len(got) != 20 || got[19] != "last" {
		t.Errorf("%d %q", len(got), got)
	}
}

func TestGateBuildFailsAndBrokenEvidence(t *testing.T) {
	h, tree := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	if out, err := commit(t, h, tree, "src/broken.go", "package src\n\nfunc X() { undefined() }\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); !strings.Contains(asJSON(t, env), "build fail: build exited 1") {
		t.Errorf("a build that does not compile: %v", env)
	}
	evidence := filepath.Join(h.root, ".athel", "changes", "CHG-001", "gates", "T-01.yaml")
	if err := os.WriteFile(evidence, []byte("schema: athel/gate-result@1\nchange: CHG-001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); errCode(env) != string(aerr.StateSchema) {
		t.Errorf("an evidence file that fails its schema: %v", env)
	}
	if err := os.WriteFile(evidence, []byte("runs: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("gate", "show", "CHG-001", "T-01"); errCode(env) != string(aerr.StateSchema) {
		t.Errorf("an unreadable evidence file: %v", env)
	}
}

func TestGateToolNotRunnable(t *testing.T) {
	h, _ := goGates(t)
	h.mustOK("gate", "baseline", "CHG-001", "T-01")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gofmt"), []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + h.env["PATH"]
	if _, env := h.runJSON("gate", "run", "CHG-001", "T-01"); !strings.Contains(asJSON(t, env), "format unavailable") {
		t.Errorf("a gate tool that cannot run: %v", env)
	}
}
