package cli

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

type harness struct {
	t     *testing.T
	root  string
	home  string
	env   map[string]string
	clock *clock.Fixed
	stdin string
	// userHome stands in for the user's home folder; empty makes UserHome fail.
	userHome string
	// cwd is the folder a command runs in; empty is root.
	cwd string
	// exe is the athel binary the git hooks call.
	exe string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{t: t, root: t.TempDir(), home: t.TempDir(), env: map[string]string{}, exe: "/opt/athel/bin/athel",
		clock: clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))}
}

func (h *harness) run(args ...string) (int, string, string) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	env := map[string]string{"ATHEL_HOME": h.home}
	for k, v := range h.env {
		env[k] = v
	}
	code := Run(Env{
		Args: args, Stdin: strings.NewReader(h.stdin), Stdout: &out, Stderr: &errOut,
		Getenv: func(k string) string { return env[k] }, Cwd: cmp.Or(h.cwd, h.root),
		Executable: func() (string, error) {
			if h.exe == "" {
				return "", errors.New("no athel binary in this test")
			}
			return h.exe, nil
		},
		UserHome: func() (string, error) {
			if h.userHome != "" {
				return h.userHome, nil
			}
			return "", errors.New("tests never use the real home")
		},
		Clock: h.clock, Version: "1.2.3",
	})
	return code, out.String(), errOut.String()
}

// runJSON runs with --json and decodes the envelope.
func (h *harness) runJSON(args ...string) (int, map[string]any) {
	h.t.Helper()
	code, out, errOut := h.run(append([]string{"--json"}, args...)...)
	if errOut != "" {
		h.t.Fatalf("stderr in --json mode: %s", errOut)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		h.t.Fatalf("stdout is not one JSON object: %q (%v)", out, err)
	}
	return code, env
}

func (h *harness) mustOK(args ...string) map[string]any {
	h.t.Helper()
	code, env := h.runJSON(args...)
	if code != 0 || env["ok"] != true {
		h.t.Fatalf("athel %v: exit %d, %v", args, code, env)
	}
	data, _ := env["data"].(map[string]any)
	return data
}

func errCode(env map[string]any) string {
	e, _ := env["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestCLIUsageErrors(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		name string
		args []string
	}{
		{"no command", nil},
		{"unknown command", []string{"deploy"}},
		{"unknown global flag", []string{"--nope", "version"}},
		{"unknown command flag", []string{"version", "--nope"}},
		{"extra argument", []string{"version", "x"}},
		{"change show without id", []string{"change", "show"}},
		{"state show two ids", []string{"state", "show", "CHG-001", "CHG-002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, errOut := h.run(tt.args...)
			if code != aerr.ExitUsage || !strings.Contains(errOut, "error:") {
				t.Errorf("exit %d stderr %q", code, errOut)
			}
			code, env := h.runJSON(tt.args...)
			if code != aerr.ExitUsage || errCode(env) != "USAGE" {
				t.Errorf("--json: exit %d env %v", code, env)
			}
		})
	}
}

func TestJSONEnvelope(t *testing.T) {
	h := newHarness(t)
	data := h.mustOK("version")
	if data["version"] != "1.2.3" || data["min_claude_code_version"] != "2.1.308" {
		t.Errorf("version data = %v", data)
	}
	code, env := h.runJSON("state", "show")
	if code != aerr.ExitState || env["ok"] != false || errCode(env) != string(aerr.StateNotInitialised) {
		t.Errorf("outside a workspace: exit %d env %v", code, env)
	}
	if e := env["error"].(map[string]any); e["next_action"] == "" || e["message"] == "" {
		t.Errorf("error lacks message/next_action: %v", e)
	}
	code, out, errOut := h.run("state", "show")
	if code != aerr.ExitState || out != "" || !strings.Contains(errOut, "next: run `athel init`") {
		t.Errorf("human error: exit %d out %q err %q", code, out, errOut)
	}
}

func TestInitIdempotent(t *testing.T) {
	h := newHarness(t)
	first := h.mustOK("init")
	if created := first["created"].([]any); len(created) != 5 {
		t.Errorf("created = %v", created)
	}
	second := h.mustOK("init")
	if created := second["created"].([]any); len(created) != 0 {
		t.Errorf("second init created %v", created)
	}
	if _, err := os.Stat(filepath.Join(h.root, ".athel", "config", "athel.yaml")); err != nil {
		t.Error(err)
	}
	code, _, _ := h.run("--workspace", filepath.Join(h.root, ".athel", "config", "athel.yaml"), "init")
	if code != aerr.ExitState {
		t.Errorf("init under a file: exit %d", code)
	}
}

func TestChangeNewFromFileTextStdin(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	prd := filepath.Join(t.TempDir(), "prd.md")
	if err := os.WriteFile(prd, []byte("# from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.stdin = "# from stdin"
	for i, args := range [][]string{
		{"--title", "File", "--prd-file", prd},
		{"--title", "Text", "--prd-text", "# from text"},
		{"--title", "Stdin", "--prd-stdin"},
	} {
		data := h.mustOK(append([]string{"change", "new"}, args...)...)
		if want := []string{"CHG-001", "CHG-002", "CHG-003"}[i]; data["id"] != want || data["state"] != "draft" {
			t.Errorf("created %v, want %s", data, want)
		}
	}
	show := h.mustOK("change", "show", "CHG-003")
	if show["created_by"] != "unknown" || show["prd"].(map[string]any)["versions"].([]any)[0].(map[string]any)["source"] != "stdin" {
		t.Errorf("change show = %v", show)
	}
	got, err := os.ReadFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "prd", "v1.md"))
	if err != nil || string(got) != "# from file" {
		t.Errorf("prd copy = %q %v", got, err)
	}
}

func TestChangeNewErrors(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	tests := []struct {
		name string
		args []string
		exit int
	}{
		{"no title", []string{"--prd-text", "x"}, aerr.ExitUsage},
		{"no prd", []string{"--title", "t"}, aerr.ExitUsage},
		{"two prd sources", []string{"--title", "t", "--prd-text", "x", "--prd-stdin"}, aerr.ExitUsage},
		{"missing prd file", []string{"--title", "t", "--prd-file", "/nonexistent/prd.md"}, aerr.ExitState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, _, _ := h.run(append([]string{"change", "new"}, tt.args...)...); code != tt.exit {
				t.Errorf("exit %d, want %d", code, tt.exit)
			}
		})
	}
}

func TestIdentityFromUserLayer(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@1\nidentity: dev@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustOK("init")
	h.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	if show := h.mustOK("change", "show", "CHG-001"); show["created_by"] != "dev@example.com" {
		t.Errorf("created_by = %v", show["created_by"])
	}
}

func TestChangeListSorted(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	for range 3 {
		h.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	}
	code, env := h.runJSON("change", "list")
	list := env["data"].([]any)
	if code != 0 || len(list) != 3 || list[0].(map[string]any)["id"] != "CHG-001" || list[2].(map[string]any)["id"] != "CHG-003" {
		t.Errorf("list = %v", list)
	}
}

func TestStateShowResolvesSingleActiveChange(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	code, env := h.runJSON("state", "show")
	if code != aerr.ExitState || !strings.Contains(env["error"].(map[string]any)["message"].(string), "0 active") {
		t.Errorf("no changes: %v", env)
	}
	h.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	view := h.mustOK("state", "show")
	if view["id"] != "CHG-001" || view["state"] != "draft" || len(view["allowed_next"].([]any)) != 3 {
		t.Errorf("view = %v", view)
	}
	h.mustOK("change", "new", "--title", "t2", "--prd-text", "p")
	if code, _ := h.runJSON("state", "show"); code != aerr.ExitState {
		t.Errorf("two active changes without an ID: exit %d", code)
	}
	if v := h.mustOK("state", "show", "CHG-002", "--json"); v["id"] != "CHG-002" {
		t.Errorf("explicit id: %v", v)
	}
	if code, _, out := h.run("state", "show", "CHG-002"); code != 0 || !strings.Contains(out, "") {
		t.Errorf("human state show: %d", code)
	}
}

func TestMinClaudeCodeVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{`{"metadata":{"athel":{"min_claude_code_version":"2.1.308"}}}`, "2.1.308", false},
		{`{"metadata":{}}`, "", true},
		{`not json`, "", true},
	}
	for _, tt := range tests {
		got, err := MinClaudeCodeVersion([]byte(tt.in))
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("MinClaudeCodeVersion(%s) = %q, %v", tt.in, got, err)
		}
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestOutputWriteFailureIsInternal(t *testing.T) {
	for _, asJSON := range []bool{true, false} {
		args := []string{"version"}
		if asJSON {
			args = append([]string{"--json"}, args...)
		}
		code := Run(Env{Args: args, Stdout: failWriter{}, Stderr: failWriter{}, Getenv: func(string) string { return "" },
			Clock: clock.System{}, Version: "x", Stdin: strings.NewReader("")})
		if code != aerr.ExitInternal {
			t.Errorf("json=%v: exit %d, want internal", asJSON, code)
		}
	}
}

func TestHumanRendering(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"plain text\n", "plain text\n"},
		{map[string]any{"b": 1, "a": "x"}, "a: x\nb: 1\n"},
	}
	for _, tt := range tests {
		got, err := human(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("human(%v) = %q, %v", tt.in, got, err)
		}
	}
	if _, err := human(func() {}); err == nil {
		t.Error("unencodable data rendered")
	}
}
