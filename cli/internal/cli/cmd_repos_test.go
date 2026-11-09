package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// devHome is a fake home folder with two clones under ~/Developer.
func devHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for name, remote := range map[string]string{"users": "git@github.com:acme/users.git", "orders": "https://github.com/acme/orders.git"} {
		dir := filepath.Join(home, "Developer", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "x"}, {"remote", "add", "origin", remote}} {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
				"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e"}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
	}
	return home
}

func TestRootsNeedTheDevelopersGrant(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	p := h.mustOK("repos", "roots", "propose")
	if len(p["current"].([]any)) != 0 || len(p["candidates"].([]any)) != 1 || p["candidates"].([]any)[0] != "~/Developer" {
		t.Fatalf("propose = %v", p)
	}
	if code, env := h.runJSON("repos", "roots", "set", "~/Developer"); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Fatalf("set without the developer's prompt: exit %d %v", code, env)
	}
	h.humanPrompt("/athel:roots ~/Developer")
	if code, _ := h.runJSON("repos", "roots", "set", "~/Developer", "~/Documents"); code != aerr.ExitPolicy {
		t.Errorf("set with other roots than granted: exit %d", code)
	}
	saved := h.mustOK("repos", "roots", "set", "~/Developer/")
	if saved["roots"].([]any)[0] != "~/Developer" {
		t.Errorf("set = %v", saved)
	}
	if code, _ := h.runJSON("repos", "roots", "set", "~/Developer"); code != aerr.ExitPolicy {
		t.Errorf("a grant is used once: exit %d", code)
	}
	if p := h.mustOK("repos", "roots", "propose"); p["current"].([]any)[0] != "~/Developer" || len(p["candidates"].([]any)) != 0 {
		t.Errorf("propose after set = %v", p)
	}
	b, _ := os.ReadFile(filepath.Join(h.home, "user.yaml"))
	if !strings.Contains(string(b), "search_roots: [~/Developer]") {
		t.Errorf("user.yaml = %s", b)
	}
}

func TestReposIndexAndResolve(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	if code, _ := h.runJSON("repos", "index"); code != aerr.ExitState {
		t.Errorf("index without roots: exit %d", code)
	}
	if code, _ := h.runJSON("repos", "show"); code != aerr.ExitState {
		t.Errorf("show without an index: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built := h.mustOK("repos", "index")
	if built["repos"] != float64(2) || built["complete"] != true {
		t.Fatalf("index = %v", built)
	}
	shown := h.mustOK("repos", "show")
	first := shown["repos"].([]any)[0].(map[string]any)
	if first["remote"] != "github.com/acme/orders" || first["fact"] != "F-0001" {
		t.Errorf("show = %v", shown)
	}
	if f := h.mustOK("fact", "show", "F-0002")["fact"].(map[string]any); f["predicate"] != "local_clone" || f["subject"].(map[string]any)["name"] != "github.com/acme/users" {
		t.Errorf("clone fact = %v", f)
	}
	h.stdin = grepDraft("users", "package users", "users")
	h.mustOK("fact", "add", "-")
	if v := h.mustOK("fact", "verify", "F-0003"); v["result"] != "verified" {
		t.Errorf("recipe resolved by repo name = %v", v)
	}
	if v := h.mustOK("fact", "verify", "F-0002"); v["result"] != "verified" {
		t.Errorf("clone fact verify = %v", v)
	}
}

func TestStaleIndexRefreshedAtSessionStart(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustOK("repos", "index")
	before := h.mustOK("repos", "show")["indexed_at"]
	h.clock.Advance(8 * 24 * time.Hour)
	payload(t, h, map[string]any{"session_id": "sess-B", "source": "startup"})
	hookOut(t, h, "session-start")
	if after := h.mustOK("repos", "show")["indexed_at"]; after == before {
		t.Error("stale index not rebuilt at session start")
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "repos", "index.yaml"), []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload(t, h, map[string]any{"session_id": "sess-C", "source": "startup"})
	hookOut(t, h, "session-start")
	if code, _ := h.runJSON("fact", "verify", "F-0001"); code != aerr.ExitState {
		t.Errorf("verify with a corrupt index: exit %d", code)
	}
}

func TestReposCommandErrors(t *testing.T) {
	h := setup(t)
	for _, args := range [][]string{{"repos", "roots", "propose", "x"}, {"repos", "roots", "set"}, {"repos", "index", "x"}, {"repos", "show", "x"}, {"repos", "roots", "set", "-bad"}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if code, _ := h.runJSON("repos", "roots", "propose"); code != aerr.ExitState {
		t.Errorf("propose without a home folder: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "index"); code != aerr.ExitState {
		t.Errorf("index without a home folder: exit %d", code)
	}
	delete(h.env, "ATHEL_SESSION_ID")
	if code, _ := h.runJSON("repos", "roots", "set", "~/a"); code != aerr.ExitState {
		t.Errorf("set without a session: exit %d", code)
	}
	outside := newHarness(t)
	for _, args := range [][]string{{"repos", "index"}, {"repos", "show"}} {
		if code, _ := outside.runJSON(args...); code != aerr.ExitState {
			t.Errorf("%v outside a workspace: exit %d", args, code)
		}
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nbogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "roots", "propose"); code != aerr.ExitPolicy {
		t.Errorf("propose with an invalid config: exit %d", code)
	}
}

func TestRootsSetErrors(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	grantsFile := filepath.Join(h.root, ".athel", "state", "grants.jsonl")
	if err := os.MkdirAll(filepath.Dir(grantsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grantsFile, []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "roots", "set", "~/Developer"); code == 0 {
		t.Error("set with a corrupt grant book succeeded")
	}
	if err := os.Remove(grantsFile); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.home, "org"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.home, "org", "athel.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/work]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.humanPrompt("/athel:roots ~/Developer")
	if code, env := h.runJSON("repos", "roots", "set", "~/Developer"); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyConfigLoosens) {
		t.Errorf("roots wider than the org's: exit %d %v", code, env)
	}
	h.humanPrompt("/athel:roots ~/work")
	audit := filepath.Join(h.root, ".athel", "audit")
	entries, _ := os.ReadDir(audit)
	for _, e := range entries {
		if err := os.WriteFile(filepath.Join(audit, e.Name()), []byte("not a record\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := h.runJSON("repos", "roots", "set", "~/work"); code == 0 {
		t.Error("set with a corrupt audit log succeeded")
	}
	noHome := setup(t)
	noHome.env["ATHEL_HOME"] = ""
	if code, _, _ := noHome.run("--json", "repos", "roots", "set", "~/a"); code != aerr.ExitState {
		t.Errorf("set without ATHEL_HOME or a home folder: exit %d", code)
	}
}

func TestIndexAndShowErrors(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	knowledge := filepath.Join(h.root, ".athel", "knowledge")
	if err := os.MkdirAll(knowledge, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowledge, "facts.jsonl"), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "index"); code != aerr.ExitState {
		t.Errorf("index with a corrupt fact cache: exit %d", code)
	}
	if err := os.MkdirAll(filepath.Join(h.root, ".athel", "repos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "repos", "index.yaml"), []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "show"); code != aerr.ExitState {
		t.Errorf("show a corrupt index: exit %d", code)
	}
}

func TestIndexSaveError(t *testing.T) {
	h := setup(t)
	h.userHome = devHome(t)
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "repos"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("repos", "index"); code != aerr.ExitState {
		t.Errorf("index into a blocked folder: exit %d", code)
	}
}

func TestCloneWithoutRemoteHasNoFact(t *testing.T) {
	h := setup(t)
	home := t.TempDir()
	dir := filepath.Join(home, "Developer", "scratch")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.userHome = home
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustOK("repos", "index")
	entry := h.mustOK("repos", "show")["repos"].([]any)[0].(map[string]any)
	if entry["remote"] != nil || entry["fact"] != nil {
		t.Errorf("entry = %v", entry)
	}
}

func TestDecideUsesConfirmedRoots(t *testing.T) {
	h := setup(t)
	home := devHome(t)
	h.userHome = home
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nrepos:\n  search_roots: [~/Developer]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.mustOK("lock", "acquire", "CHG-001")
	users := filepath.Join(home, "Developer", "users")
	fetch := h.decide("Bash", map[string]any{"command": "git -C " + users + " fetch"}, h.root)
	if fetch["decision"] != "deny" || fetch["rule"] != "12" {
		t.Errorf("fetch in a developer clone = %v", fetch)
	}
	read := h.decide("Read", map[string]any{"file_path": filepath.Join(users, "main.go")}, h.root)
	if read["decision"] != "allow" {
		t.Errorf("read in a developer clone = %v", read)
	}
}
