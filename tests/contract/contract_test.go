// Package contract proves FR-DISC-008: a new tool is added with a tools.yaml entry and an
// optional playbook, with no change to Athel's code. The tool is a fake MCP server the shop
// fixture provides (shopmetrics), which no built-in entry knows.
package contract

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func build(t *testing.T, dir, pkg string) string {
	t.Helper()
	bin := filepath.Join(dir, filepath.Base(pkg))
	if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v %s", pkg, err, out)
	}
	return bin
}

type athel struct {
	t   *testing.T
	bin string
	ws  string
	env []string
}

func (a athel) run(args ...string) (string, int) {
	a.t.Helper()
	cmd := exec.Command(a.bin, append([]string{"--workspace", a.ws}, args...)...)
	cmd.Env = a.env
	out, err := cmd.Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok { //nolint:errorlint // Output returns *exec.ExitError directly
		code = ee.ExitCode()
	}
	return string(out), code
}

func (a athel) decide(tool string) string {
	a.t.Helper()
	out, code := a.run("--json", "policy", "decide", "--tool", tool, "--input", "{}", "--cwd", a.ws, "--agent", "athel:scout")
	var env struct {
		Data struct {
			Decision string `json:"decision"`
		} `json:"data"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil {
		a.t.Fatalf("policy decide %s: exit %d %s", tool, code, out)
	}
	return env.Data.Decision
}

const entry = `schema: athel/tools@1
tools:
  - id: shopmetrics
    kind: mcp
    match: {server: shopmetrics}
    purpose: [metrics]
    read: [query]
    write: []
    policy:
      prod: {read: allow, write: deny}
    playbook: playbooks/shopmetrics.md
`

func TestNewToolByRegistryOnly(t *testing.T) {
	dir := t.TempDir()
	a := athel{t: t, bin: build(t, dir, "github.com/mr-vaibh/athel/cli/cmd/athel"), ws: filepath.Join(dir, "ws"),
		env: []string{"HOME=" + filepath.Join(dir, "home"), "ATHEL_HOME=" + filepath.Join(dir, "athel-home"), "ATHEL_SESSION_ID=contract"}}
	if err := os.MkdirAll(a.ws, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init"}, {"change", "new", "--title", "c", "--prd-text", "p"}, {"lock", "acquire", "CHG-001"}} {
		if out, code := a.run(args...); code != 0 {
			t.Fatalf("%v: %d %s", args, code, out)
		}
	}
	if a.decide("mcp__shopmetrics__query") != "deny" {
		t.Fatal("an unregistered MCP server must be denied (FR-DISC-007)")
	}
	cfg := filepath.Join(a.ws, ".athel", "config")
	if err := os.MkdirAll(filepath.Join(cfg, "playbooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "tools.yaml"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "playbooks", "shopmetrics.md"), []byte("# shopmetrics playbook\nUse query with a metric name.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := a.decide("mcp__shopmetrics__query"); got != "allow" {
		t.Errorf("registered read tool = %s, want allow", got)
	}
	if got := a.decide("mcp__shopmetrics__drop_series"); got == "allow" {
		t.Error("a tool not on the read list was allowed")
	}
	if out, code := a.run("playbook", "show", "shopmetrics"); code != 0 || !strings.Contains(out, "shopmetrics playbook") {
		t.Errorf("playbook show = %d %q", code, out)
	}
	assertServerAnswers(t, build(t, dir, "github.com/mr-vaibh/athel/tests/fixtures/shop/cmd/shopmcp"))
}

func assertServerAnswers(t *testing.T, bin string) {
	t.Helper()
	cmd := exec.Command(bin, "shopmetrics")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"query","arguments":{"metric":"kafka_consumergroup_lag"}}}` + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	lines := 0
	for sc.Scan() {
		lines++
	}
	if lines != 2 || !strings.Contains(string(out), "notifications") {
		t.Errorf("server replies = %s", out)
	}
}
