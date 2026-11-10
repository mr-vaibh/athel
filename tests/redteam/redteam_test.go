// Package redteam is M1's exit gate EC-1/EC-2 (doc 04 §Red-team suite, doc 08 M1): every case in
// cases/*.yaml runs through the real athel binary (`athel policy decide`) against a fixture
// workspace and must produce exactly the expected decision and rule. Forbidden cases must never
// come back allow or approve; allow-list cases prove the guard is not won by denying everything.
package redteam

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/internal/m3docs"
)

// Groups are doc 04's seven red-team groups; allow is the allow-list suite.
var Groups = []string{"prod-writes", "cloud-writes", "secrets", "git", "injection", "scope", "obfuscation"}

// minPerGroup and minTotal are the coverage floor (EC-1: ≥ 200 variants across the groups).
const (
	minPerGroup = 15
	minTotal    = 200
)

// Expect is a case's required outcome.
type Expect struct {
	Decision string `yaml:"decision"`
	Rule     string `yaml:"rule"`
}

// Case is one tool call. Bash calls give command; other tools give input. Placeholders {tree},
// {ws} and {home} are replaced with the fixture paths.
type Case struct {
	Name    string         `yaml:"name"`
	Tool    string         `yaml:"tool"`
	Command string         `yaml:"command"`
	Input   map[string]any `yaml:"input"`
	Session string         `yaml:"session"` // "guard": a session that holds no change lock
	Agent   string         `yaml:"agent"`
	Wrap    bool           `yaml:"wrap"` // also run the command through every wrapper
	Expect  Expect         `yaml:"expect"`
	group   string
}

// File is one cases/*.yaml file.
type File struct {
	Group string `yaml:"group"`
	Cases []Case `yaml:"cases"`
}

// wrappers hide a command inside shell constructs the parser must see through (FR-SAFE-009).
var wrappers = []struct{ name, format, avoid string }{
	{"bash -c", "bash -c '%s'", "'"},
	{"sh -c", `sh -c "%s"`, `"$`},
	{"sudo", "sudo %s", ""},
	{"env prefix", "env DEBUG=1 %s", ""},
	{"timeout", "timeout 60 %s", ""},
	{"nohup", "nohup %s", ""},
	{"subshell", "( %s )", ""},
	{"and-chain", "true && %s", ""},
	{"sequence", "ls; %s", ""},
	{"or-fallback", "%s || true", ""},
	{"eval", "eval '%s'", "'"},
	{"function", "f() { %s; }; f", ""},
	{"for loop", "for i in 1 2; do %s; done", ""},
	{"if", "if true; then %s; fi", ""},
}

// expand returns the case itself plus one variant per applicable wrapper.
func expand(c Case) []Case {
	out := []Case{c}
	if !c.Wrap {
		return out
	}
	for _, w := range wrappers {
		if w.avoid != "" && strings.ContainsAny(c.Command, w.avoid) {
			continue
		}
		v := c
		v.Name, v.Command = c.Name+" ["+w.name+"]", fmt.Sprintf(w.format, c.Command)
		out = append(out, v)
	}
	return out
}

func load(t *testing.T, pattern string) []Case {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("cases", pattern))
	if err != nil || len(files) == 0 {
		t.Fatalf("no case files %s: %v", pattern, err)
	}
	var all []Case
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		var cf File
		if err := dec.Decode(&cf); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, c := range cf.Cases {
			c.group = cf.Group
			if c.Tool == "" {
				c.Tool = "Bash"
			}
			all = append(all, expand(c)...)
		}
	}
	return all
}

// fixture is a workspace with change CHG-001 in building, an approved plan for repo svc
// (src/**), its worktree, and a home folder whose kube, gcloud and aws settings point at prod.
type fixture struct {
	bin, ws, home, tree string
	env                 []string
}

const session = "sess-redteam"

func (f *fixture) run(t *testing.T, stdin string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(f.bin, args...)
	cmd.Env, cmd.Dir, cmd.Stdin = f.env, f.ws, strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("athel %v: %v: %s", args, err, out)
	}
	return out
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{bin: filepath.Join(dir, "athel"), ws: filepath.Join(dir, "ws"), home: filepath.Join(dir, "home")}
	f.tree = filepath.Join(f.ws, "repos", "svc@CHG-001")
	if out, err := exec.Command("go", "build", "-o", f.bin, "github.com/mr-vaibh/athel/cli/cmd/athel").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	// A minimal environment: never the real HOME, kube, gcloud or aws settings of this machine.
	f.env = []string{"HOME=" + f.home, "ATHEL_HOME=" + filepath.Join(dir, "athel-home"), "ATHEL_SESSION_ID=" + session}
	for path, body := range fixtureFiles {
		write(t, filepath.Join(dir, path), body)
	}
	f.run(t, "", "init")
	f.run(t, "", "change", "new", "--title", "Red team", "--prd-text", m3docs.PRD)
	// The M3 step guards need the spec step's work before specified and planned (ADR 021).
	f.run(t, m3docs.Review, "prd", "review", "save", "CHG-001", "-")
	f.run(t, m3docs.Conventions, "conventions", "save", m3docs.Repo, "-", "--new")
	f.run(t, m3docs.Spec, "spec", "save", "CHG-001", "-")
	f.run(t, "", "plan", "set", "CHG-001", filepath.Join(dir, "plan.yaml"))
	for _, to := range []string{"investigating", "specified", "awaiting_decisions", "planned"} {
		f.run(t, "", "state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "red team")
	}
	prompt, _ := json.Marshal(map[string]any{"session_id": session, "prompt_id": "p-1", "prompt": "/athel:approve CHG-001", "cwd": f.ws})
	f.run(t, string(prompt), "hook", "user-prompt-submit")
	f.run(t, "", "state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "red team")
	f.run(t, "", "state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "red team")
	return f
}

func (f *fixture) expandVars(s string) string {
	return strings.NewReplacer("{tree}", f.tree, "{ws}", f.ws, "{home}", f.home).Replace(s)
}

type decision struct {
	Decision string `json:"decision"`
	Rule     string `json:"rule"`
	Code     string `json:"code"`
	Reason   string `json:"reason"`
}

func (f *fixture) decide(t *testing.T, c Case) decision {
	t.Helper()
	input := c.Input
	if c.Tool == "Bash" {
		input = map[string]any{"command": f.expandVars(c.Command)}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--json", "policy", "decide", "--tool", c.Tool, "--input", f.expandVars(string(raw)), "--cwd", f.tree}
	if c.Session == "guard" {
		args = append([]string{"--session", "sess-other"}, args...)
	}
	if c.Agent != "" {
		args = append(args, "--agent", c.Agent)
	}
	var env struct {
		OK   bool     `json:"ok"`
		Data decision `json:"data"`
	}
	out := f.run(t, "", args...)
	if err := json.Unmarshal(out, &env); err != nil || !env.OK {
		t.Fatalf("decide %s: %v %s", c.Name, err, out)
	}
	return env.Data
}

// maxDecision bounds one `athel policy decide` process. A slow decision path matters because a
// PreToolUse hook that times out is a failure (blocked by onFailure, ADR 016) rather than a
// decision; the hook timeout is 10 s and the latency budget 50 ms at p95.
const maxDecision = 2 * time.Second

func check(t *testing.T, f *fixture, cases []Case, forbidden bool) {
	t.Helper()
	for _, c := range cases {
		start := time.Now()
		got := f.decide(t, c)
		if took := time.Since(start); took > maxDecision {
			t.Errorf("[%s] %s: decision took %s", c.group, c.Name, took)
		}
		if forbidden && (got.Decision == "allow" || got.Decision == "approve" || got.Decision == "none") {
			t.Errorf("FALSE ALLOW [%s] %s: %s rule %s (%s)", c.group, c.Name, got.Decision, got.Rule, got.Reason)
			continue
		}
		if got.Decision != c.Expect.Decision || got.Rule != c.Expect.Rule {
			t.Errorf("[%s] %s: got %s rule %s, want %s rule %s (%s)", c.group, c.Name, got.Decision, got.Rule, c.Expect.Decision, c.Expect.Rule, got.Reason)
		}
	}
}

func TestRedTeamSuite(t *testing.T) {
	cases := load(t, "[^_]*.yaml")
	for _, c := range cases {
		if c.Expect.Decision != "deny" {
			t.Errorf("[%s] %s: red-team cases must expect deny, not %q", c.group, c.Name, c.Expect.Decision)
		}
	}
	check(t, newFixture(t), cases, true)
}

func TestAllowListSuite(t *testing.T) {
	check(t, newFixture(t), load(t, "_allow.yaml"), false)
}

func TestCoveragePerGroup(t *testing.T) {
	counts := map[string]int{}
	total := 0
	for _, c := range load(t, "[^_]*.yaml") {
		if !slices.Contains(Groups, c.group) {
			t.Errorf("case %s has unknown group %q", c.Name, c.group)
		}
		counts[c.group]++
		total++
	}
	for _, g := range Groups {
		if counts[g] < minPerGroup {
			t.Errorf("group %s has %d cases, want ≥ %d", g, counts[g], minPerGroup)
		}
	}
	if total < minTotal {
		t.Errorf("%d red-team cases, want ≥ %d", total, minTotal)
	}
	t.Logf("red-team cases per group: %v (total %d)", counts, total)
}
