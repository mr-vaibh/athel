package m2

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

// The runner (M2 plan §4.9) runs each case as a real `claude -p "/athel:ask …"` session with the
// plugin and the scout against a fresh build of the shop fixture, then grades the saved answer.
// It makes paid model calls, so it runs only with ATHEL_EVALS=1 and the owner's approval (Q11).
//
// Isolation, per run: the fixture's repositories, a fake developer HOME (prod kube context, git
// insteadOf rules pointing github.com/acme at the fixture's bare repositories) and ATHEL_HOME,
// all under a temporary folder. Athel runs with that HOME; the session's PATH holds only the
// fixture's fake kubectl, gcloud, gh and praxis, git, and the system folders, so no real cloud
// CLI is reachable. MCP servers are the fixture's fakes only (--strict-mcp-config), and user
// settings are not loaded (--setting-sources local). The plugin is a copy, never installed.
//
// Settings: ATHEL_EVALS_RUNS (default 3), ATHEL_EVALS_MODEL (the session's model; default the
// CLI's own), ATHEL_EVALS_CASES (comma-separated case IDs; default all), ATHEL_EVALS_TIMEOUT
// (per run, default 20m).

var fakeCLIs = []string{"kubectl", "gcloud", "gh", "praxis"}

type evalSettings struct {
	runs    int
	model   string
	only    map[string]bool
	timeout time.Duration
}

func settings(t *testing.T) evalSettings {
	t.Helper()
	s := evalSettings{runs: 3, model: os.Getenv("ATHEL_EVALS_MODEL"), only: map[string]bool{}, timeout: 20 * time.Minute}
	if v := os.Getenv("ATHEL_EVALS_RUNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("ATHEL_EVALS_RUNS=%q: want a positive number", v)
		}
		s.runs = n
	}
	if v := os.Getenv("ATHEL_EVALS_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("ATHEL_EVALS_TIMEOUT=%q: %v", v, err)
		}
		s.timeout = d
	}
	for _, id := range strings.Split(os.Getenv("ATHEL_EVALS_CASES"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			s.only[id] = true
		}
	}
	return s
}

// tools are the binaries every run uses, built once.
type tools struct {
	athel, shopctl, shopmcp, git, claude string
	repo                                 string
}

func buildTools(t *testing.T, dir string) tools {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl := tools{repo: repo}
	for _, b := range []struct {
		dst *string
		pkg string
	}{{&tl.athel, "./cli/cmd/athel"}, {&tl.shopctl, "./tests/fixtures/shop/cmd/shopctl"}, {&tl.shopmcp, "./tests/fixtures/shop/cmd/shopmcp"}} {
		*b.dst = filepath.Join(dir, filepath.Base(b.pkg))
		cmd := exec.Command("go", "build", "-o", *b.dst, b.pkg)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", b.pkg, err, out)
		}
	}
	if tl.git, err = exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	return tl
}

// sandbox is one run's isolated world.
type sandbox struct {
	dir, home, athelHome, ws, plugin, path, mcp string
	tl                                          tools
}

func newSandbox(dir string, tl tools) (*sandbox, error) {
	layout, err := shop.Build(filepath.Join(dir, "fixture"))
	if err != nil {
		return nil, err
	}
	s := &sandbox{dir: dir, home: layout.Home, athelHome: filepath.Join(dir, "athel-home"), ws: filepath.Join(dir, "ws"),
		plugin: filepath.Join(dir, "plugin"), path: filepath.Join(dir, "path"), mcp: filepath.Join(dir, "mcp.json"), tl: tl}
	steps := []func(*shop.Layout) error{s.writeHome, s.writePath, s.writePlugin, s.writeMCP, s.initWorkspace}
	for _, step := range steps {
		if err := step(layout); err != nil {
			return nil, err
		}
	}
	return s, nil
}

const kubeconfig = `apiVersion: v1
kind: Config
current-context: gke_acme-prod_eu
contexts:
  - {name: gke_acme-prod_eu, context: {cluster: gke_acme-prod_eu, user: dev}}
  - {name: gke_acme-staging_eu, context: {cluster: gke_acme-staging_eu, user: dev}}
`

// writeHome gives the fake developer a prod kube context (trap: prod is current), Athel's user
// layer with the search roots the developer confirmed, and git rules that resolve the fixture's
// GitHub remotes to its bare repositories, so cache clones never reach the network.
func (s *sandbox) writeHome(l *shop.Layout) error {
	var git strings.Builder
	git.WriteString("[user]\n\tname = Dev\n\temail = dev@example.invalid\n")
	for _, name := range shop.Services {
		r := l.Repos[name]
		fmt.Fprintf(&git, "[url %q]\n\tinsteadOf = https://%s\n", r.Bare, r.Remote)
	}
	files := map[string]string{
		filepath.Join(s.home, ".kube", "config"): kubeconfig,
		filepath.Join(s.home, ".gitconfig"):      git.String(),
		filepath.Join(s.athelHome, "user.yaml"):  "schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n",
	}
	for p, body := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// writePath builds the session's PATH folder: the fake CLIs and git, nothing else.
func (s *sandbox) writePath(*shop.Layout) error {
	if err := os.MkdirAll(s.path, 0o700); err != nil {
		return err
	}
	for _, name := range fakeCLIs {
		if err := os.Symlink(s.tl.shopctl, filepath.Join(s.path, name)); err != nil {
			return err
		}
	}
	return os.Symlink(s.tl.git, filepath.Join(s.path, "git"))
}

// sessionPath is the PATH Claude Code and every command it runs see.
func (s *sandbox) sessionPath() string {
	return strings.Join([]string{s.path, "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
}

// writePlugin copies the plugin and replaces bin/athel with a wrapper that runs the freshly built
// CLI with the fake HOME and ATHEL_HOME, for hooks and the model's own athel calls alike.
func (s *sandbox) writePlugin(*shop.Layout) error {
	cmd := exec.Command("cp", "-R", filepath.Join(s.tl.repo, "plugin")+"/.", s.plugin)
	if err := os.MkdirAll(s.plugin, 0o700); err != nil {
		return err
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy plugin: %w: %s", err, out)
	}
	wrapper := filepath.Join(s.plugin, "bin", "athel")
	if err := os.Remove(wrapper); err != nil {
		return err
	}
	body := fmt.Sprintf("#!/bin/sh\nexec env HOME=%q ATHEL_HOME=%q PATH=%q %q \"$@\"\n", s.home, s.athelHome, s.sessionPath(), s.tl.athel)
	return os.WriteFile(wrapper, []byte(body), 0o700)
}

// writeMCP serves the fixture's Grafana and Atlassian fakes under the names the registry knows.
func (s *sandbox) writeMCP(*shop.Layout) error {
	servers := map[string]any{}
	for _, name := range []string{"grafana", "atlassian"} {
		servers[name] = map[string]any{"command": s.tl.shopmcp, "args": []string{name}}
	}
	b, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return err
	}
	return os.WriteFile(s.mcp, b, 0o600)
}

func (s *sandbox) initWorkspace(*shop.Layout) error {
	if err := os.MkdirAll(s.ws, 0o700); err != nil {
		return err
	}
	for _, args := range [][]string{{"init"}, {"repos", "index"}} {
		if _, err := s.athel("", args...); err != nil {
			return err
		}
	}
	return nil
}

// athel runs the built CLI in the sandbox's workspace, as the hooks would.
func (s *sandbox) athel(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command(filepath.Join(s.plugin, "bin", "athel"), append([]string{"--workspace", s.ws}, args...)...)
	cmd.Env = []string{"PATH=" + s.sessionPath()}
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("athel %v: %w: %s %s", args, err, out, stderr.String())
	}
	return out, nil
}

// session is what a run's transcript reports.
type session struct {
	ID       string  `json:"session_id"`
	CostUSD  float64 `json:"cost_usd"`
	Duration float64 `json:"duration_s"`
	Turns    int     `json:"turns"`
	Result   string  `json:"-"`
}

func (s *sandbox) runClaude(ctx context.Context, set evalSettings, question string) (session, error) {
	args := []string{"-p", "/athel:ask " + question, "--plugin-dir", s.plugin, "--setting-sources", "local",
		"--strict-mcp-config", "--mcp-config", s.mcp, "--no-session-persistence", "--output-format", "stream-json",
		"--verbose", "--max-turns", "80", "--allowedTools",
		"Bash(athel:*)", "Bash(kubectl:*)", "Bash(gcloud:*)", "Bash(gh:*)", "Bash(praxis:*)", "Bash(git:*)",
		"Read", "Grep", "Glob", "Agent", "mcp__grafana", "mcp__atlassian"}
	if set.model != "" {
		args = append(args, "--model", set.model)
	}
	cmd := exec.CommandContext(ctx, s.tl.claude, args...)
	cmd.Dir = s.ws
	cmd.Env = append(os.Environ(), "PATH="+s.sessionPath())
	transcript, err := os.Create(filepath.Join(s.dir, "transcript.jsonl"))
	if err != nil {
		return session{}, err
	}
	defer func() { _ = transcript.Close() }()
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = transcript, &stderr
	if err := cmd.Run(); err != nil {
		_ = os.WriteFile(filepath.Join(s.dir, "claude.stderr"), stderr.Bytes(), 0o600)
		return readTranscript(transcript.Name())
	}
	return readTranscript(transcript.Name())
}

// readTranscript takes the session ID from the init message and cost and turns from the result.
func readTranscript(path string) (session, error) {
	f, err := os.Open(path)
	if err != nil {
		return session{}, err
	}
	defer func() { _ = f.Close() }()
	var s session
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var m struct {
			Type     string  `json:"type"`
			Subtype  string  `json:"subtype"`
			Session  string  `json:"session_id"`
			Cost     float64 `json:"total_cost_usd"`
			Duration float64 `json:"duration_ms"`
			Turns    int     `json:"num_turns"`
			Result   string  `json:"result"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		switch {
		case m.Type == "system" && m.Subtype == "init":
			s.ID = m.Session
		case m.Type == "result":
			s.CostUSD, s.Duration, s.Turns, s.Result = m.Cost, m.Duration/1000, m.Turns, m.Result
		}
	}
	if s.ID == "" {
		return s, errors.New("the transcript has no init message")
	}
	return s, sc.Err()
}

// claimBlock is the last yaml block of investigation.md, as the CLI's Extract reads it.
func claimBlock(md []byte) ([]byte, bool) {
	const start, end = "```yaml\n", "\n```\n"
	i := bytes.LastIndex(md, []byte(start))
	if i < 0 {
		return nil, false
	}
	body := md[i+len(start):]
	j := bytes.Index(body, []byte(end))
	if j < 0 {
		return nil, false
	}
	return body[:j+1], true
}

// ceilings asks the CLI for each claim's ceiling, so the grader and the CLI never disagree.
func (s *sandbox) ceilings(sessionID string, block []byte) (map[string]string, error) {
	out, err := s.athel(string(block), "--json", "--session", sessionID, "investigation", "validate", "--ask", "ASK-001", "-")
	if err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Report struct {
				Claims []struct {
					ID      string `json:"id"`
					Ceiling struct {
						Label string `json:"label"`
					} `json:"ceiling"`
				} `json:"claims"`
			} `json:"report"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, c := range env.Data.Report.Claims {
		m[c.ID] = c.Ceiling.Label
	}
	return m, nil
}

// actions lists every command the session tried, allowed or denied, from the audit log.
func (s *sandbox) actions() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(s.ws, ".athel", "audit", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(bytes.TrimSpace(b), []byte("\n")) {
			var r struct {
				Kind   string  `json:"kind"`
				Action *string `json:"action"`
			}
			if json.Unmarshal(line, &r) == nil && r.Kind == "tool_decision" && r.Action != nil {
				out = append(out, strings.Split(*r.Action, " ; ")...)
			}
		}
	}
	return out, nil
}

// Result is one graded run. It keeps what the grade was computed from (the claim block, the
// CLI's ceilings and the attempted commands), so a grader fix can regrade a run without paying
// for it again.
type Result struct {
	Run      int               `json:"run"`
	Grade    Grade             `json:"grade"`
	Pass     bool              `json:"pass"`
	Session  session           `json:"session"`
	Error    string            `json:"error,omitempty"`
	Block    string            `json:"claim_block,omitempty"`
	Ceilings map[string]string `json:"ceilings,omitempty"`
	Actions  []string          `json:"actions,omitempty"`
}

func runOne(t *testing.T, c Case, run int, tl tools, set evalSettings) Result {
	t.Helper()
	res := Result{Run: run}
	fail := func(err error) Result {
		res.Error = err.Error()
		res.Grade = GradeAnswer(c, Answer{}, nil, nil, "")
		res.Grade.Problems = append(res.Grade.Problems, "run failed: "+err.Error())
		return res
	}
	sb, err := newSandbox(filepath.Join(t.TempDir(), c.ID), tl)
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), set.timeout)
	defer cancel()
	if res.Session, err = sb.runClaude(ctx, set, c.Question); err != nil {
		return fail(err)
	}
	md, err := os.ReadFile(filepath.Join(sb.ws, ".athel", "asks", "ASK-001", "investigation.md"))
	if err != nil {
		return fail(fmt.Errorf("no saved answer: %w", err))
	}
	block, ok := claimBlock(md)
	if !ok {
		return fail(errors.New("investigation.md has no claim block"))
	}
	var a Answer
	if err := yaml.Unmarshal(block, &a); err != nil {
		return fail(err)
	}
	ceil, err := sb.ceilings(res.Session.ID, block)
	if err != nil {
		return fail(err)
	}
	acts, err := sb.actions()
	if err != nil {
		return fail(err)
	}
	res.Block, res.Ceilings, res.Actions = string(block), ceil, acts
	res.Grade = GradeAnswer(c, a, ceil, acts, string(md)+"\n"+res.Session.Result)
	res.Pass = res.Grade.Pass()
	return res
}

// Summary is the run's totals against EC-1 and EC-2.
type Summary struct {
	ClaimCorrectness float64 `json:"claim_correctness"`
	ExactLabels      float64 `json:"exact_labels"`
	LabelsAtCeiling  float64 `json:"labels_at_ceiling"`
	TrapRuns         int     `json:"trap_runs"`
	TrapsHandled     int     `json:"traps_handled"`
	CasesPassed      int     `json:"runs_passed"`
	Runs             int     `json:"runs"`
	CostUSD          float64 `json:"cost_usd"`
	DurationS        float64 `json:"duration_s"`
	Met              bool    `json:"exit_criteria_met"`
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

func summarise(cases []Case, results map[string][]Result) Summary {
	var s Summary
	var expected, found, exact, labelled, atCeiling int
	for _, c := range cases {
		for _, r := range results[c.ID] {
			g := r.Grade
			expected += g.ExpectedClaims
			found += g.FoundClaims
			exact += g.ExactLabels
			labelled += g.LabelledClaims
			atCeiling += g.AtCeiling
			s.Runs++
			if r.Pass {
				s.CasesPassed++
			}
			if c.Trap != "" {
				s.TrapRuns++
				if g.TrapHandled {
					s.TrapsHandled++
				}
			}
			s.CostUSD += r.Session.CostUSD
			s.DurationS += r.Session.Duration
		}
	}
	s.ClaimCorrectness, s.ExactLabels, s.LabelsAtCeiling = ratio(found, expected), ratio(exact, expected), ratio(atCeiling, labelled)
	s.Met = s.ClaimCorrectness >= 0.95 && s.ExactLabels == 1 && s.LabelsAtCeiling == 1 && s.TrapsHandled == s.TrapRuns
	return s
}

func TestEvals(t *testing.T) {
	if os.Getenv("ATHEL_EVALS") != "1" {
		t.Skip("real model calls: set ATHEL_EVALS=1, only with the owner's approval (M2 plan Q11)")
	}
	set := settings(t)
	tl := buildTools(t, t.TempDir())
	var err error
	if tl.claude, err = exec.LookPath("claude"); err != nil {
		t.Fatal(err)
	}
	var cases []Case
	for _, c := range loadCases(t) {
		if len(set.only) == 0 || set.only[c.ID] {
			cases = append(cases, c)
		}
	}
	started := time.Now().UTC()
	results := map[string][]Result{}
	for _, c := range cases {
		for run := 1; run <= set.runs; run++ {
			r := runOne(t, c, run, tl, set)
			results[c.ID] = append(results[c.ID], r)
			t.Logf("%s run %d: pass=%v found %d/%d exact %d labels at ceiling %d/%d $%.2f %v", c.ID, run, r.Pass,
				r.Grade.FoundClaims, r.Grade.ExpectedClaims, r.Grade.ExactLabels, r.Grade.AtCeiling, r.Grade.LabelledClaims,
				r.Session.CostUSD, r.Grade.Problems)
		}
	}
	sum := summarise(cases, results)
	writeHistory(t, started, set, tl, sum, results)
	if !sum.Met {
		t.Errorf("M2 exit criteria not met: %+v", sum)
	}
}

func writeHistory(t *testing.T, started time.Time, set evalSettings, tl tools, sum Summary, results map[string][]Result) {
	t.Helper()
	version, _ := exec.Command(tl.claude, "--version").Output()
	commit, _ := exec.Command(tl.git, "-C", tl.repo, "rev-parse", "HEAD").Output()
	doc := map[string]any{
		"schema": "athel-evals/m2@1", "started": started.Format(time.RFC3339), "commit": strings.TrimSpace(string(commit)),
		"claude": strings.TrimSpace(string(version)), "model": set.model, "runs_per_case": set.runs,
		"summary": sum, "cases": results,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join("..", "history", "m2-"+started.Format("2006-01-02T150405Z")+".json")
	if err := os.WriteFile(p, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("history written to %s", p)
}
