package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// approvedOnUsers takes the cache fixture's change to plan_approved with a plan whose one task
// is in the indexed repo users.
func approvedOnUsers(t *testing.T) *harness {
	t.Helper()
	h, _, _ := approvedOnUsersRemote(t)
	return h
}

// approvedOnUsersRemote is approvedOnUsers plus the folder of bare remotes and a git runner, so
// a test can push someone else's work.
func approvedOnUsersRemote(t *testing.T) (*harness, string, func(string, ...string)) {
	t.Helper()
	h, _, src, git := cacheFixture(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	m3Spec(t, h)
	h.mustOK("plan", "set", "CHG-001", writeTemp(t, "users-plan.yaml", strings.NewReplacer("repo: r,", "repo: users,", "verbs: [edit, test]", "verbs: [edit, test, commit, push]").Replace(m3PlanDoc)))
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	h.humanPrompt("/athel:approve CHG-001")
	h.mustOK("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok")
	return h, src, git
}

func TestGitWorktree(t *testing.T) {
	h := approvedOnUsers(t)
	v := h.mustOK("git", "worktree", "CHG-001", "users")
	tree := filepath.Join(h.root, "repos", "users@CHG-001")
	if v["branch"] != "athel/CHG-001-phone-update" || v["created"] != true || v["path"] != tree || v["remote"] != "github.com/acme/users" {
		t.Fatalf("worktree = %v", v)
	}
	if _, err := os.Stat(filepath.Join(tree, ".git")); err != nil {
		t.Fatalf("no worktree at %s: %v", tree, err)
	}
	if again := h.mustOK("git", "worktree", "CHG-001", "users"); again["created"] != false || again["branch"] != v["branch"] {
		t.Errorf("a second call returns the same worktree: %v", again)
	}
	repos := h.mustOK("state", "show", "CHG-001")["repos"].([]any)
	if len(repos) != 1 || repos[0].(map[string]any)["worktree"] != "repos/users@CHG-001" {
		t.Errorf("change.yaml repos = %v", repos)
	}
	// Rule 9 now knows the branch Athel created: pushing it is Athel's own, any other is not.
	push := func(branch string) map[string]any {
		return h.decide("Bash", map[string]any{"command": "git push origin " + branch}, tree)
	}
	if d := push("athel/CHG-001-phone-update"); d["decision"] != "allow" || d["rule"] != "9" {
		t.Errorf("own branch push = %v", d)
	}
	if d := push("athel/CHG-009-other"); d["decision"] != "deny" || d["code"] != string(aerr.PolicyForeignBranch) {
		t.Errorf("foreign branch push = %v", d)
	}
}

func TestGitWorktreeRefusals(t *testing.T) {
	h, _, _, _ := cacheFixture(t)
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "plan_approved or building") {
		t.Errorf("before approval: %d %v", code, env)
	}
	h = approvedOnUsers(t)
	if code, env := h.runJSON("git", "worktree", "CHG-001", "orders"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "no task of the approved plan") {
		t.Errorf("a repo outside the plan: %d %v", code, env)
	}
	if code, _ := h.runJSON("git", "worktree", "CHG-001"); code != aerr.ExitUsage {
		t.Errorf("missing repo: %d", code)
	}
	if code, _ := h.runJSON("git", "worktree", "CHG-009", "users"); code != aerr.ExitState {
		t.Errorf("unknown change: %d", code)
	}
	delete(h.env, "ATHEL_SESSION_ID")
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %d %v", code, env)
	}
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	if err := os.MkdirAll(filepath.Join(h.root, "repos", "users@CHG-001"), 0o700); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("git", "worktree", "CHG-001", "users"); errCode(env) != string(aerr.ConflictWorktree) {
		t.Errorf("a path in the way: %d %v", code, env)
	}
}

// treeSum hashes every file under dir (its .git folder too): path, mode and content.
func treeSum(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s %v %d\n", strings.TrimPrefix(p, dir), info.Mode(), info.ModTime().UnixNano())
		if d.Type().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			_, _ = h.Write(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestDeveloperCloneUnchanged (FR-GIT-001): making Athel's worktree, which seeds the cache from
// the developer's clone, leaves that clone byte for byte as it was, timestamps included.
func TestDeveloperCloneUnchanged(t *testing.T) {
	h := approvedOnUsers(t)
	clone := filepath.Join(h.userHome, "Developer", "users")
	before := treeSum(t, clone)
	h.mustOK("git", "worktree", "CHG-001", "users")
	if after := treeSum(t, clone); after != before {
		t.Error("the developer's clone changed")
	}
}

// fakeGH puts a gh on the harness's PATH that answers every call with prs (JSON).
func fakeGH(t *testing.T, h *harness, prs string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncat <<'EOF'\n" + prs + "\nEOF\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

const teammatePR = `[{"number":812,"author":{"login":"teammate"},"headRefName":"teammate/phone","files":[{"path":"src/a.go"}]}]`

func TestGitPreflightOverlapRaisesCard(t *testing.T) {
	h := approvedOnUsers(t)
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "r"); code == 0 || !strings.Contains(asJSON(t, env), "no git preflight for users") {
		t.Fatalf("building before any preflight: %d %v", code, env)
	}
	fakeGH(t, h, teammatePR)
	v := h.mustOK("git", "preflight", "CHG-001", "users")
	card, _ := v["card"].(string)
	if v["clean"] != false || card == "" || v["branch"] != "athel/CHG-001-phone-update" || !strings.HasSuffix(v["report"].(string), "preflight/users.md") {
		t.Fatalf("preflight = %v", v)
	}
	md, err := os.ReadFile(v["report"].(string))
	if err != nil || !strings.Contains(string(md), "pr #812 by teammate: src/a.go") {
		t.Errorf("report: %v\n%s", err, md)
	}
	if again := h.mustOK("git", "preflight", "CHG-001", "users"); again["card"] != card {
		t.Errorf("a re-run finds its card instead of raising another: %v", again)
	}
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "r"); code == 0 || !strings.Contains(asJSON(t, env), card+" is unanswered and blocks the build step") {
		t.Fatalf("building with the card open: %d %v", code, env)
	}
	h.humanPrompt("/athel:decide CHG-001 " + card + "=b")
	h.mustOK("decide", "answer", "CHG-001", card+"=b")
	h.mustOK("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "preflight done")
}

func TestGitPreflightClean(t *testing.T) {
	h := approvedOnUsers(t)
	fakeGH(t, h, `[{"number":9,"author":"someone","headRefName":"docs","files":["docs/x.md"]}]`)
	v := h.mustOK("git", "preflight", "CHG-001", "users")
	if v["clean"] != true || v["card"] != nil {
		t.Fatalf("preflight = %v", v)
	}
	h.mustOK("state", "transition", "CHG-001", "--to", "building", "--by", "athel", "--reason", "clear")
	if again := h.mustOK("git", "preflight", "CHG-001", "users"); again["clean"] != true {
		t.Errorf("preflight runs again while building: %v", again)
	}
}

func TestGitPreflightErrors(t *testing.T) {
	h := approvedOnUsers(t)
	fakeGH(t, h, "not json")
	if code, env := h.runJSON("git", "preflight", "CHG-001", "users"); code != aerr.ExitTool || errCode(env) != string(aerr.ToolUnreachable) {
		t.Errorf("gh output unreadable: %d %v", code, env)
	}
	h.env["PATH"] = t.TempDir()
	if code, _ := h.runJSON("git", "preflight", "CHG-001", "users"); code == 0 {
		t.Error("without gh or git the preflight fails")
	}
	for _, args := range [][]string{{"git", "preflight", "CHG-001"}, {"git", "preflight", "CHG-009", "users"}} {
		if code, _ := h.runJSON(args...); code == 0 {
			t.Errorf("%v must fail", args)
		}
	}
}

func TestGitPreflightPushedBranch(t *testing.T) {
	h, src, git := approvedOnUsersRemote(t)
	mate := filepath.Join(t.TempDir(), "mate")
	git(filepath.Dir(mate), "clone", "-q", filepath.Join(src, "users.git"), mate)
	git(mate, "switch", "-q", "-c", "spike/phone")
	if err := os.MkdirAll(filepath.Join(mate, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mate, "src", "phone.go"), []byte("package src\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(mate, "add", "-A")
	git(mate, "-c", "user.name=Mate", "-c", "user.email=mate@example.invalid", "commit", "-q", "-m", "spike")
	git(mate, "push", "-q", "origin", "spike/phone")
	fakeGH(t, h, "[]")
	v := h.mustOK("git", "preflight", "CHG-001", "users")
	if v["clean"] != false || !strings.Contains(asJSON(t, v), `"ref":"spike/phone"`) || !strings.Contains(asJSON(t, v), "src/phone.go") {
		t.Errorf("a recently pushed branch on planned paths blocks: %v", v)
	}
}

func TestGitPreflightToolFailures(t *testing.T) {
	h := approvedOnUsers(t)
	bin := t.TempDir()
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'HTTP 401: Bad credentials' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("git", "preflight", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) || !strings.Contains(asJSON(t, env), "Bad credentials") {
		t.Errorf("gh failing: %v", env)
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("git", "preflight", "CHG-001", "users"); errCode(env) != string(aerr.ToolUnreachable) {
		t.Errorf("gh not runnable: %v", env)
	}
	fakeGH(t, h, "[]")
	if err := os.RemoveAll(filepath.Join(h.root, "cache")); err != nil {
		t.Fatal(err)
	}
	if _, env := h.runJSON("git", "preflight", "CHG-001", "users"); errCode(env) != string(aerr.ToolGit) {
		t.Errorf("the cache clone gone: %v", env)
	}
}
