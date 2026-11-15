// Package sandbox proves doc 08's practice: the helper CLI has no network access of its own
// (git, gh, govulncheck and health checks are separate programs it runs), enforced by a test
// that runs it under a network-deny sandbox.
package sandbox

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/tests/internal/m3docs"
)

// netPackages open sockets; net/url and net/netip only parse text.
var netPackages = []string{"net", "net/http", "net/rpc", "net/smtp", "crypto/tls", "net/http/httptrace"}

// TestCLILinksNoNetworkCode: the athel binary cannot open a socket itself, on any platform.
func TestCLILinksNoNetworkCode(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/mr-vaibh/athel/cli/cmd/athel").Output()
	if err != nil {
		t.Fatal(err)
	}
	deps := strings.Fields(string(out))
	for _, p := range netPackages {
		if slices.Contains(deps, p) {
			t.Errorf("the athel binary links %s", p)
		}
	}
}

// denyNet returns the command that runs argv with network access denied, or "" with why the
// platform has no such sandbox. Unix sockets stay open: the migration gate's PostgreSQL uses one.
func denyNet(t *testing.T) ([]string, string) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return []string{"sandbox-exec", "-p", "(version 1)(allow default)(deny network*)(allow network* (remote unix-socket))"}, ""
		}
	case "linux":
		if exec.Command("unshare", "-rn", "true").Run() == nil {
			return []string{"unshare", "-rn"}, ""
		}
	}
	return nil, "no unprivileged network-deny sandbox on this machine (sandbox-exec on macOS, unshare -rn on Linux)"
}

type athel struct {
	t       *testing.T
	prefix  []string
	bin, ws string
	env     []string
}

func (a *athel) run(stdin string, args ...string) map[string]any {
	a.t.Helper()
	argv := append(slices.Clone(a.prefix), append([]string{a.bin, "--json", "--workspace", a.ws}, args...)...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Stdin = a.env, strings.NewReader(stdin)
	out, err := cmd.Output()
	var env map[string]any
	if jerr := json.Unmarshal(out, &env); err != nil || jerr != nil || env["ok"] != true {
		a.t.Fatalf("athel %v under the sandbox: %v\n%s", args, err, out)
	}
	data, _ := env["data"].(map[string]any)
	return data
}

// TestCLIMakesNoNetworkCalls runs a change from init to an approved plan, with hook decisions
// and the audit check, under the sandbox: every step works with the network denied.
func TestCLIMakesNoNetworkCalls(t *testing.T) {
	prefix, why := denyNet(t)
	if prefix == nil {
		t.Skip(why + "; TestCLILinksNoNetworkCode still holds")
	}
	control(t, prefix)
	ws := t.TempDir()
	bin := filepath.Join(t.TempDir(), "athel")
	if out, err := exec.Command("go", "build", "-o", bin, "github.com/mr-vaibh/athel/cli/cmd/athel").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	a := &athel{t: t, prefix: prefix, bin: bin, ws: ws, env: []string{"HOME=" + filepath.Join(ws, ".user"),
		"ATHEL_HOME=" + filepath.Join(ws, ".home"), "ATHEL_SESSION_ID=s1", "PATH=" + os.Getenv("PATH")}}
	a.run("", "init")
	a.run("", "change", "new", "--title", "Phone update", "--prd-text", "# PRD\nLet customers update their phone number.")
	a.run("", "state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	a.run(m3docs.Review, "prd", "review", "save", "CHG-001", "-")
	a.run(m3docs.Conventions, "conventions", "save", m3docs.Repo, "-", "--new")
	a.run(m3docs.Spec, "spec", "save", "CHG-001", "-")
	plan := filepath.Join(t.TempDir(), "plan.yaml")
	if err := os.WriteFile(plan, []byte(m3docs.Plan("svc", []string{"src/**"}, []string{"edit", "test"})), 0o600); err != nil {
		t.Fatal(err)
	}
	a.run("", "plan", "set", "CHG-001", plan)
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		a.run("", "state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	for _, args := range [][]string{{"trace", "CHG-001"}, {"plan", "summary", "CHG-001"}, {"review", "rules"},
		{"policy", "validate"}, {"audit", "verify"}, {"resume", "CHG-001"}} {
		a.run("", args...)
	}
}

// control proves the sandbox denies a connection: to a listener of this test on 127.0.0.1,
// which bash reaches without the sandbox and cannot reach with it. Nothing leaves the machine.
func control(t *testing.T, prefix []string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dial := []string{"bash", "-c", fmt.Sprintf("exec 3<>/dev/tcp/127.0.0.1/%d", l.Addr().(*net.TCPAddr).Port)}
	if out, err := exec.Command(dial[0], dial[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("the control cannot connect even without the sandbox: %v %s", err, out)
	}
	argv := append(slices.Clone(prefix), dial...)
	if exec.Command(argv[0], argv[1:]...).Run() == nil {
		t.Fatal("the sandbox let a connection through")
	}
}
