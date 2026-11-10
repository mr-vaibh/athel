// Package evalkit is the eval harness the M2 and M3 evals share (M3 plan §5): settings, the
// binaries a run needs, the isolated sandbox around the shop fixture, Claude Code sessions in it,
// and what a session's transcript and audit log report.
//
// Isolation, per run: the fixture's repositories, a fake developer HOME (prod kube context, git
// insteadOf rules pointing github.com/acme at the fixture's bare repositories) and ATHEL_HOME,
// all under a temporary folder. Athel runs with that HOME; the session's PATH holds only the
// fixture's fake kubectl, gcloud, gh and praxis, git, and the system folders, so no real cloud
// CLI is reachable. MCP servers are the fixture's fakes only (--strict-mcp-config), and user
// settings are not loaded (--setting-sources local). The plugin is a copy, never installed.
package evalkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

// FakeCLIs are the cloud CLIs the fixture fakes; the session's PATH has no real ones.
var FakeCLIs = []string{"kubectl", "gcloud", "gh", "praxis"}

// Settings are a run's knobs, from the environment: ATHEL_EVALS_RUNS (default 3),
// ATHEL_EVALS_MODEL (the session's model; default the CLI's own), ATHEL_EVALS_CASES
// (comma-separated case IDs; default all) and ATHEL_EVALS_TIMEOUT (per session, default 20m).
type Settings struct {
	Runs    int
	Model   string
	Only    map[string]bool
	Timeout time.Duration
}

// LoadSettings reads Settings through getenv, so tests need no real environment.
func LoadSettings(getenv func(string) string) (Settings, error) {
	s := Settings{Runs: 3, Model: getenv("ATHEL_EVALS_MODEL"), Only: map[string]bool{}, Timeout: 20 * time.Minute}
	if v := getenv("ATHEL_EVALS_RUNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return s, fmt.Errorf("ATHEL_EVALS_RUNS=%q: want a positive number", v)
		}
		s.Runs = n
	}
	if v := getenv("ATHEL_EVALS_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return s, fmt.Errorf("ATHEL_EVALS_TIMEOUT=%q: %w", v, err)
		}
		s.Timeout = d
	}
	for _, id := range strings.Split(getenv("ATHEL_EVALS_CASES"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			s.Only[id] = true
		}
	}
	return s, nil
}

// Selected reports whether a case runs under these settings.
func (s Settings) Selected(id string) bool {
	return len(s.Only) == 0 || s.Only[id]
}

// Tools are the binaries every run uses, built once. Claude is found by the caller, only for
// paid runs.
type Tools struct {
	Athel, Shopctl, Shopmcp, Git, Claude string
	Repo                                 string
}

// BuildTools builds the CLI and the fixture's fakes from the repository at repo into dir.
func BuildTools(repo, dir string) (Tools, error) {
	tl := Tools{Repo: repo}
	for _, b := range []struct {
		dst *string
		pkg string
	}{{&tl.Athel, "./cli/cmd/athel"}, {&tl.Shopctl, "./tests/fixtures/shop/cmd/shopctl"}, {&tl.Shopmcp, "./tests/fixtures/shop/cmd/shopmcp"}} {
		*b.dst = filepath.Join(dir, filepath.Base(b.pkg))
		cmd := exec.Command("go", "build", "-o", *b.dst, b.pkg) //nolint:gosec // fixed package list, output in the caller's folder
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			return tl, fmt.Errorf("build %s: %w\n%s", b.pkg, err, out)
		}
	}
	var err error
	tl.Git, err = exec.LookPath("git")
	return tl, err
}

// Sandbox is one run's isolated world.
type Sandbox struct {
	Dir, Home, AthelHome, WS, Plugin, Path, MCP string
	Tools                                       Tools
}

// NewSandbox builds the fixture under dir and wires the fake HOME, PATH, plugin copy, MCP fakes
// and an initialised workspace with the repo index.
func NewSandbox(dir string, tl Tools) (*Sandbox, error) {
	layout, err := shop.Build(filepath.Join(dir, "fixture"))
	if err != nil {
		return nil, err
	}
	s := &Sandbox{Dir: dir, Home: layout.Home, AthelHome: filepath.Join(dir, "athel-home"), WS: filepath.Join(dir, "ws"),
		Plugin: filepath.Join(dir, "plugin"), Path: filepath.Join(dir, "path"), MCP: filepath.Join(dir, "mcp.json"), Tools: tl}
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

// writeHome gives the fake developer prod as the current kube context and gcloud project (trap:
// prod is the default), Athel's user layer with the search roots the developer confirmed, and
// git rules that resolve the fixture's GitHub remotes to its bare repositories, so cache clones
// never reach the network.
func (s *Sandbox) writeHome(l *shop.Layout) error {
	var git strings.Builder
	git.WriteString("[user]\n\tname = Dev\n\temail = dev@example.invalid\n")
	for _, name := range shop.Services {
		r := l.Repos[name]
		fmt.Fprintf(&git, "[url %q]\n\tinsteadOf = https://%s\n", r.Bare, r.Remote)
	}
	files := map[string]string{
		filepath.Join(s.Home, ".kube", "config"):                                       kubeconfig,
		filepath.Join(s.Home, ".config", "gcloud", "active_config"):                    "default\n",
		filepath.Join(s.Home, ".config", "gcloud", "configurations", "config_default"): "[core]\nproject = acme-prod\naccount = dev@example.invalid\n",
		filepath.Join(s.Home, ".gitconfig"):                                            git.String(),
		filepath.Join(s.AthelHome, "user.yaml"):                                        "schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n",
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
func (s *Sandbox) writePath(*shop.Layout) error {
	if err := os.MkdirAll(s.Path, 0o700); err != nil {
		return err
	}
	for _, name := range FakeCLIs {
		if err := os.Symlink(s.Tools.Shopctl, filepath.Join(s.Path, name)); err != nil {
			return err
		}
	}
	return os.Symlink(s.Tools.Git, filepath.Join(s.Path, "git"))
}

// SessionPath is the PATH Claude Code and every command it runs see.
func (s *Sandbox) SessionPath() string {
	return strings.Join([]string{s.Path, "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
}

// writePlugin copies the plugin and replaces bin/athel with a wrapper that runs the freshly built
// CLI with the fake HOME and ATHEL_HOME, for hooks and the model's own athel calls alike.
func (s *Sandbox) writePlugin(*shop.Layout) error {
	if err := os.MkdirAll(s.Plugin, 0o700); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-R", filepath.Join(s.Tools.Repo, "plugin")+"/.", s.Plugin) //nolint:gosec // copies this repository's plugin into the sandbox
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy plugin: %w: %s", err, out)
	}
	// A fresh checkout has no built bin/athel (it is ignored by git); a developer's may.
	wrapper := filepath.Join(s.Plugin, "bin", "athel")
	if err := os.Remove(wrapper); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	body := fmt.Sprintf("#!/bin/sh\nexec env HOME=%q ATHEL_HOME=%q PATH=%q %q \"$@\"\n", s.Home, s.AthelHome, s.SessionPath(), s.Tools.Athel)
	return os.WriteFile(wrapper, []byte(body), 0o700) //nolint:gosec // the wrapper must be executable
}

// writeMCP serves the fixture's Grafana and Atlassian fakes under the names the registry knows.
func (s *Sandbox) writeMCP(*shop.Layout) error {
	servers := map[string]any{}
	for _, name := range []string{"grafana", "atlassian"} {
		servers[name] = map[string]any{"command": s.Tools.Shopmcp, "args": []string{name}}
	}
	b, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return err
	}
	return os.WriteFile(s.MCP, b, 0o600)
}

func (s *Sandbox) initWorkspace(*shop.Layout) error {
	if err := os.MkdirAll(s.WS, 0o700); err != nil {
		return err
	}
	for _, args := range [][]string{{"init"}, {"repos", "index"}} {
		if _, err := s.Athel("", args...); err != nil {
			return err
		}
	}
	return nil
}

// Athel runs the built CLI in the sandbox's workspace, as the hooks would.
func (s *Sandbox) Athel(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command(filepath.Join(s.Plugin, "bin", "athel"), append([]string{"--workspace", s.WS}, args...)...) //nolint:gosec // the sandbox's own CLI wrapper
	cmd.Env = []string{"PATH=" + s.SessionPath()}
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("athel %v: %w: %s %s", args, err, out, stderr.String())
	}
	return out, nil
}

// Session is what a session's transcript reports.
type Session struct {
	ID       string  `json:"session_id"`
	CostUSD  float64 `json:"cost_usd"`
	Duration float64 `json:"duration_s"`
	Turns    int     `json:"turns"`
	Result   string  `json:"-"`
}

// Prompt is one `claude -p` session: the prompt and the transcript file name under the
// sandbox's folder.
type Prompt struct {
	Text       string
	Transcript string
	Model      string
}

// allowedTools are what a session may use without asking; Athel's hooks still decide each call.
var allowedTools = []string{"Bash(athel:*)", "Bash(kubectl:*)", "Bash(gcloud:*)", "Bash(gh:*)", "Bash(praxis:*)", "Bash(git:*)",
	"Bash(ls:*)", "Bash(cat:*)", "Bash(head:*)", "Bash(grep:*)", "Bash(find:*)", "Bash(wc:*)",
	"Read", "Grep", "Glob", "Agent", "mcp__grafana", "mcp__atlassian"}

// RunClaude runs one session in the sandbox and reads its transcript. A session that exits with
// an error still reports what its transcript holds; its stderr is kept beside the transcript.
func (s *Sandbox) RunClaude(ctx context.Context, p Prompt) (Session, error) {
	// The developer's clones live outside the workspace, as on a real machine; --add-dir lets
	// Claude Code's own file tools read them.
	args := []string{"-p", p.Text, "--plugin-dir", s.Plugin, "--setting-sources", "local",
		"--strict-mcp-config", "--mcp-config", s.MCP, "--no-session-persistence", "--output-format", "stream-json",
		"--verbose", "--max-turns", "80", "--add-dir", filepath.Join(s.Home, "Developer"), "--allowedTools"}
	args = append(args, allowedTools...)
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	cmd := exec.CommandContext(ctx, s.Tools.Claude, args...) //nolint:gosec // the claude binary the caller found on PATH
	cmd.Dir = s.WS
	cmd.Env = append(os.Environ(), "PATH="+s.SessionPath())
	path := filepath.Join(s.Dir, p.Transcript)
	transcript, err := os.Create(path) //nolint:gosec // a file in the run's own temporary folder
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = transcript.Close() }()
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = transcript, &stderr
	if err := cmd.Run(); err != nil {
		if werr := os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".stderr", stderr.Bytes(), 0o600); werr != nil {
			return Session{}, werr
		}
	}
	return ReadTranscript(path)
}

// ReadTranscript takes the session ID from the init message and cost and turns from the result.
func ReadTranscript(path string) (Session, error) {
	f, err := os.Open(path) //nolint:gosec // a transcript this harness wrote
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = f.Close() }()
	var s Session
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

// Actions lists every command the sessions tried, allowed or denied, from the audit log.
func (s *Sandbox) Actions() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(s.WS, ".athel", "audit", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // the sandbox's own audit log
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

// LastYAMLBlock is the last fenced yaml block of a Markdown file the CLI rendered, as its own
// Extract reads it: investigation.md's claims, blast-radius.md's report, a PRD review's block.
func LastYAMLBlock(md []byte) ([]byte, bool) {
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

// Ratio is a/b, and 1 when nothing was expected.
func Ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

// WriteHistory saves a run's record as <dir>/<prefix>-<UTC start>.json, with the commit and the
// Claude Code version it ran against, and returns the path.
func WriteHistory(dir, prefix string, started time.Time, tl Tools, doc map[string]any) (string, error) {
	version, _ := exec.Command(tl.Claude, "--version").Output()                    //nolint:gosec // the claude binary found on PATH
	commit, _ := exec.Command(tl.Git, "-C", tl.Repo, "rev-parse", "HEAD").Output() //nolint:gosec // git on PATH
	doc["started"] = started.Format(time.RFC3339)
	doc["commit"] = strings.TrimSpace(string(commit))
	doc["claude"] = strings.TrimSpace(string(version))
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, prefix+"-"+started.Format("2006-01-02T150405Z")+".json")
	return p, os.WriteFile(p, append(b, '\n'), 0o600)
}
