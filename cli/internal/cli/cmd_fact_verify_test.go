package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// committedRepo makes a git repository with one committed file, using no system or global config.
func committedRepo(t *testing.T, file, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "x"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func grepDraft(repo, pattern, object string) string {
	return `subject: {kind: event, name: UserUpdated}
predicate: consumed_by
object: {kind: service, name: ` + object + `}
env: prod
confidence: likely
sources: [{kind: code, ref: crm-sync/src/consumers/user.ts:1}]
verify: [{tool: git, action: grep, repo: "` + repo + `", pattern: "` + pattern + `"}]
freshness_class: topology
`
}

func TestFactVerifyCommand(t *testing.T) {
	h := setup(t)
	repo := committedRepo(t, "user.ts", "subscribe('users.updated.v1')\n")
	h.stdin = grepDraft(repo, "users.updated.v1", "crm-sync")
	h.mustOK("fact", "add", "-")
	v := h.mustOK("fact", "verify", "F-0001", "--change", "CHG-001")
	if v["result"] != "verified" || len(v["flagged"].([]any)) != 0 {
		t.Errorf("verify = %v", v)
	}
	h.stdin = grepDraft(repo, "users.deleted.v1", "notification-service")
	h.mustOK("fact", "add", "-")
	h.mustOK("fact", "use", "F-0002", "--purpose", "orientation", "--change", "CHG-001")
	d := h.mustOK("fact", "verify", "F-0002")
	if d["result"] != "disputed" || len(d["flagged"].([]any)) != 1 || d["flagged"].([]any)[0] != "CHG-001" {
		t.Fatalf("disputed verify = %v", d)
	}
	added := h.mustOK("fact", "add", "-")
	if added["created"] != true || added["fact"].(map[string]any)["supersedes"] != "F-0002" || len(added["flagged"].([]any)) != 1 {
		t.Errorf("superseding add = %v", added)
	}
	flags := h.mustOK("change", "show", "CHG-001")["flags"].([]any)
	if len(flags) != 2 || flags[0].(map[string]any)["kind"] != "fact_disputed" ||
		flags[1].(map[string]any)["kind"] != "fact_superseded" || flags[1].(map[string]any)["replacement"] != "F-0003" ||
		!strings.Contains(flags[0].(map[string]any)["reason"].(string), "git grep") {
		t.Errorf("change flags = %v", flags)
	}
}

func TestFactVerifyFromObservation(t *testing.T) {
	h := setup(t)
	h.stdin = `subject: {kind: service, name: crm-sync}
predicate: replicas
object: {kind: value, name: "2"}
env: prod
confidence: likely
sources: [{kind: cluster, ref: "deploy/crm-sync", tool_use_id: toolu_k}]
verify: [{tool: kubectl, action: get, pattern: "crm-sync 2/2"}]
freshness_class: runtime
`
	h.mustOK("fact", "add", "-")
	dir := filepath.Join(h.root, ".athel", "knowledge", "observations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	obs := `{"schema":"athel/observation@1","tool_use_id":"toolu_k","session":"s","agent_type":"athel:scout","tool":"Bash",` +
		`"input":"{\"command\":\"kubectl get deploy crm-sync --context prod\"}","output":"{\"stdout\":\"crm-sync 2/2\"}",` +
		`"output_sha256":"sha256:` + strings.Repeat("0", 64) + `","truncated":false,"suspicious":[],"at":"2026-11-01T10:00:00.000Z","expires_at":"2026-11-08T10:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(dir, "toolu_k.json"), []byte(obs), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := h.mustOK("fact", "verify", "F-0001", "--observed", "0=toolu_k"); v["result"] != "verified" {
		t.Errorf("verify from observation = %v", v)
	}
	if v := h.mustOK("fact", "verify", "F-0001"); v["result"] != "unverified" {
		t.Errorf("verify without the observation = %v", v)
	}
}

func TestFactVerifyErrors(t *testing.T) {
	h := setup(t)
	h.stdin = grepDraft("relative/repo", "x", "crm-sync")
	h.mustOK("fact", "add", "-")
	if v := h.mustOK("fact", "verify", "F-0001"); v["result"] != "unverified" {
		t.Errorf("repo not in the index = %v", v)
	}
	for _, args := range [][]string{{"fact", "verify"}, {"fact", "verify", "F-0001", "--observed", "x"}, {"fact", "verify", "F-0001", "--observed", "0="}} {
		if code, _ := h.runJSON(args...); code != aerr.ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	if code, _ := h.runJSON("fact", "verify", "F-0042"); code != aerr.ExitState {
		t.Errorf("unknown fact: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("fact", "verify", "F-0001"); code != aerr.ExitState {
		t.Errorf("outside a workspace: exit %d", code)
	}
	tools := filepath.Join(h.root, ".athel", "config", "tools.yaml")
	if err := os.WriteFile(tools, []byte("schema: athel/tools@1\ntools: [{id: x}]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("fact", "verify", "F-0001"); code != aerr.ExitPolicy {
		t.Errorf("invalid policy: exit %d", code)
	}
	if err := os.Remove(tools); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@2\nbogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("fact", "verify", "F-0001"); code != aerr.ExitPolicy {
		t.Errorf("invalid config: exit %d", code)
	}
}

func TestFlagDependentsErrors(t *testing.T) {
	h := setup(t)
	repo := committedRepo(t, "user.ts", "x\n")
	h.stdin = grepDraft(repo, "absent-pattern", "crm-sync")
	h.mustOK("fact", "add", "-")
	for _, title := range []string{"deleted later", "edited by hand later"} {
		h.mustOK("change", "new", "--title", title, "--prd-text", "p")
	}
	for _, ch := range []string{"CHG-001", "CHG-002", "CHG-003"} {
		h.mustOK("fact", "use", "F-0001", "--purpose", "orientation", "--change", ch)
	}
	if err := os.RemoveAll(filepath.Join(h.root, ".athel", "changes", "CHG-002")); err != nil {
		t.Fatal(err)
	}
	cf := filepath.Join(h.root, ".athel", "changes", "CHG-003", "change.yaml")
	cb, _ := os.ReadFile(cf)
	if err := os.WriteFile(cf, []byte(strings.Replace(string(cb), "title: edited by hand later", "title: edited", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("fact", "verify", "F-0001"); code != aerr.ExitIntegrity {
		t.Errorf("flagging a hand-edited change: exit %d %v", code, env)
	}
	if v := h.mustOK("change", "show", "CHG-001"); len(v["flags"].([]any)) != 1 {
		t.Errorf("CHG-001 flags = %v; the deleted CHG-002 is skipped before CHG-003 fails", v["flags"])
	}
	access := filepath.Join(h.root, ".athel", "knowledge", "access.jsonl")
	if err := os.WriteFile(access, []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.stdin = grepDraft(repo, "absent-pattern", "crm-sync")
	if code, _ := h.runJSON("fact", "add", "-"); code != aerr.ExitState {
		t.Errorf("superseding with a corrupt access log: exit %d", code)
	}
}

func TestScriptCallsAreNotRead(t *testing.T) {
	h := setup(t)
	h.stdin = strings.Replace(grepDraft("x", "y", "crm-sync"), `verify: [{tool: git, action: grep, repo: "x", pattern: "y"}]`,
		`verify: [{tool: kubectl, action: get, pattern: "crm-sync"}]`, 1)
	h.mustOK("fact", "add", "-")
	dir := filepath.Join(h.root, ".athel", "knowledge", "observations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	obs := `{"schema":"athel/observation@1","tool_use_id":"toolu_s","session":"s","agent_type":"athel:scout","tool":"Bash",` +
		`"input":"{\"command\":\"./show.sh\"}","output":"{\"stdout\":\"crm-sync\"}","output_sha256":"sha256:` + strings.Repeat("0", 64) +
		`","truncated":false,"suspicious":[],"at":"2026-11-01T10:00:00.000Z","expires_at":"2026-11-08T10:00:00.000Z"}`
	if err := os.WriteFile(filepath.Join(dir, "toolu_s.json"), []byte(obs), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := h.mustOK("fact", "verify", "F-0001", "--observed", "0=toolu_s"); v["result"] != "unverified" {
		t.Errorf("a script call is not evidence that kubectl ran: %v", v)
	}
}
