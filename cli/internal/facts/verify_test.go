package facts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/observe"
)

// gitRepo makes a repository with one commit holding files, plus uncommitted edits. Git runs
// with no system or global config, like RunGit.
func gitRepo(t *testing.T, committed, uncommitted map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write := func(files map[string]string) {
		for p, body := range files {
			full := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	run("init", "-q", "-b", "main")
	write(committed)
	run("add", "-A")
	run("commit", "-q", "-m", "fixture")
	run("remote", "add", "origin", "https://github.com/acme/crm-sync.git")
	write(uncommitted)
	return dir
}

func recipeFor(dir string) Recipe {
	return Recipe{Repo: func(name string) (string, error) {
		if name == "crm-sync" {
			return dir, nil
		}
		return "", errors.New("not in the repo index")
	}}
}

func TestGitRecipeSteps(t *testing.T) {
	dir := gitRepo(t,
		map[string]string{"src/consumers/user.ts": "subscribe('users.updated.v1')\n", "README.md": "crm-sync\n"},
		map[string]string{"src/consumers/order.ts": "subscribe('orders.created.v1')\n"})
	r := recipeFor(dir)
	tests := []struct {
		name string
		step Step
		want string
	}{
		{"grep present", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "users.updated.v1"}, Match},
		{"grep in path", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "users.updated.v1", "path": "src/consumers/"}, Match},
		{"grep wrong path", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "users.updated.v1", "path": "docs/"}, Mismatch},
		{"uncommitted content never counts", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "orders.created.v1"}, Mismatch},
		{"grep absent", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "users.deleted.v1", "expect": "absent"}, Match},
		{"exists", Step{"tool": "git", "action": "exists", "repo": "crm-sync", "path": "src/consumers/user.ts"}, Match},
		{"missing file", Step{"tool": "git", "action": "exists", "repo": "crm-sync", "path": "src/consumers/order.ts"}, Mismatch},
		{"remote", Step{"tool": "git", "action": "remote", "repo": "crm-sync", "url": "https://github.com/acme/crm-sync.git"}, Match},
		{"other remote", Step{"tool": "git", "action": "remote", "repo": "crm-sync", "url": "https://github.com/acme/fork.git"}, Mismatch},
		{"unknown repo", Step{"tool": "git", "action": "grep", "repo": "payments", "pattern": "x"}, Unreachable},
		{"no pattern", Step{"tool": "git", "action": "grep", "repo": "crm-sync"}, Unreachable},
		{"unknown action", Step{"tool": "git", "action": "push", "repo": "crm-sync"}, Unreachable},
		{"unknown expect", Step{"tool": "git", "action": "grep", "repo": "crm-sync", "pattern": "x", "expect": "maybe"}, Unreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Check(tt.step, 0); got.Outcome != tt.want {
				t.Errorf("outcome = %+v, want %s", got, tt.want)
			}
		})
	}
}

func TestGitFailuresAreUnreachable(t *testing.T) {
	notRepo := t.TempDir()
	r := Recipe{Repo: func(string) (string, error) { return notRepo, nil }}
	for _, step := range []Step{
		{"tool": "git", "action": "grep", "repo": "x", "pattern": "y"},
		{"tool": "git", "action": "remote", "repo": "x", "url": "y"},
	} {
		if got := r.Check(step, 0); got.Outcome != Unreachable || !strings.Contains(got.Detail, "git exit") {
			t.Errorf("not a repo: %+v", got)
		}
	}
	broken := Recipe{Repo: r.Repo, Git: func(string, ...string) (int, string, error) { return 0, "", errors.New("git not installed") }}
	if got := broken.Check(Step{"tool": "git", "action": "grep", "repo": "x", "pattern": "y"}, 0); got.Outcome != Unreachable || !strings.Contains(got.Detail, "not installed") {
		t.Errorf("git missing: %+v", got)
	}
	if code, _, err := RunGit(filepath.Join(notRepo, "none")); err != nil || code == 0 {
		t.Errorf("git in a missing folder: exit %d %v", code, err)
	}
}

// observed returns a Recipe whose captured calls are the given observations.
func observed(obs map[string]observe.Observation, steps map[int]string) Recipe {
	return Recipe{
		Observation: func(id string) (observe.Observation, error) {
			if o, ok := obs[id]; ok {
				return o, nil
			}
			return observe.Observation{}, aerr.New(aerr.StateNotFound, "no captured output for tool call "+id)
		},
		ToolsOf: func(tool string, input []byte) []string {
			if strings.Contains(string(input), "kubectl") {
				return []string{"kubectl"}
			}
			return []string{strings.TrimPrefix(tool, "mcp__")}
		},
		Observed: steps,
	}
}

func TestObservedRecipeSteps(t *testing.T) {
	obs := map[string]observe.Observation{
		"toolu_k":   {Tool: "Bash", Input: `{"command":"kubectl get deploy -n shop"}`, Output: `{"stdout":"crm-sync 2/2\n\"quoted\" name"}`},
		"toolu_g":   {Tool: "mcp__grafana", Input: `{}`, Output: `{"series":[]}`},
		"toolu_cut": {Tool: "Bash", Input: `{"command":"kubectl get pods"}`, Output: `{"stdout":"aaa`, Truncated: true},
	}
	step := func(tool, pattern, expect string) Step {
		return Step{"tool": tool, "action": "get", "pattern": pattern, "expect": expect}
	}
	tests := []struct {
		name  string
		step  Step
		steps map[int]string
		want  string
	}{
		{"present", step("kubectl", "crm-sync", ""), map[int]string{0: "toolu_k"}, Match},
		{"quoted text matched unescaped", step("kubectl", `"quoted" name`, ""), map[int]string{0: "toolu_k"}, Match},
		{"absent but present", step("kubectl", "crm-sync", "absent"), map[int]string{0: "toolu_k"}, Mismatch},
		{"present but absent", step("grafana", "crm-sync", ""), map[int]string{0: "toolu_g"}, Mismatch},
		{"no observation named", step("kubectl", "x", ""), nil, Unreachable},
		{"observation missing", step("kubectl", "x", ""), map[int]string{0: "toolu_none"}, Unreachable},
		{"wrong tool", step("grafana", "crm-sync", ""), map[int]string{0: "toolu_k"}, Unreachable},
		{"no pattern", step("kubectl", "", ""), map[int]string{0: "toolu_k"}, Unreachable},
		{"cut output proves nothing", step("kubectl", "zzz", ""), map[int]string{0: "toolu_cut"}, Unreachable},
		{"cut output still shows a match", step("kubectl", "aaa", ""), map[int]string{0: "toolu_cut"}, Match},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := observed(obs, tt.steps).Check(tt.step, 0); got.Outcome != tt.want {
				t.Errorf("outcome = %+v, want %s", got, tt.want)
			}
		})
	}
}

func checkAll(outcome string) func(Step, int) StepResult {
	return func(_ Step, i int) StepResult { return StepResult{Step: i, Outcome: outcome} }
}

func TestRecipeAllMatchActive(t *testing.T) {
	s, clk := newStore(t)
	f := mustAdd(t, s, draft(t))
	clk.Advance(30 * time.Hour)
	res, err := s.Verify(f.ID, checkAll(Match), &Ref{Change: "CHG-001"}, "sess")
	if err != nil || res.Result != ResultVerified || res.Fact.Status != Active || len(res.Steps) != 1 {
		t.Fatalf("verify = %+v %v", res, err)
	}
	if u, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "sess"); err != nil || u.Result != InWindow {
		t.Errorf("use after verify = %+v %v", u, err)
	}
	recs := readAccess(t, s)
	if len(recs) != 2 || !strings.Contains(recs[0], `"result":"verified"`) || !strings.Contains(recs[0], `"purpose":"decision"`) {
		t.Errorf("access log = %v", recs)
	}
}

func TestUnreachableLeavesFactLogsUnverified(t *testing.T) {
	s, clk := newStore(t)
	f := mustAdd(t, s, draft(t))
	clk.Advance(30 * time.Hour)
	res, err := s.Verify(f.ID, checkAll(Unreachable), &Ref{Ask: "ASK-001"}, "")
	if err != nil || res.Result != ResultUnverified || res.Fact.Status != Active || res.Fact.VerifiedAt != f.VerifiedAt {
		t.Fatalf("verify = %+v %v", res, err)
	}
	if recs := readAccess(t, s); len(recs) != 1 || !strings.Contains(recs[0], `"result":"unverified"`) {
		t.Errorf("access log = %v", recs)
	}
	if _, err := s.Use(f.ID, Decision, Ref{Ask: "ASK-001"}, ""); !aerr.HasCode(err, aerr.StateFactStale) {
		t.Errorf("a fact that could not be re-verified must stay stale: %v", err)
	}
}

func TestMismatchDisputesAndFindsDependents(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	for _, ch := range []string{"CHG-002", "CHG-001", "CHG-002"} {
		if _, err := s.Use(f.ID, Orientation, Ref{Change: ch}, "s"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Use(f.ID, Orientation, Ref{Ask: "ASK-001"}, "s"); err != nil {
		t.Fatal(err)
	}
	res, err := s.Verify(f.ID, checkAll(Mismatch), &Ref{Change: "CHG-001"}, "s")
	if err != nil || res.Result != ResultDisputed || res.Fact.Status != Disputed {
		t.Fatalf("verify = %+v %v", res, err)
	}
	deps, err := s.Dependents(f.ID)
	if err != nil || strings.Join(deps, ",") != "CHG-001,CHG-002" {
		t.Errorf("dependents = %v %v", deps, err)
	}
	if _, err := s.Use(f.ID, Decision, Ref{Change: "CHG-001"}, "s"); !aerr.HasCode(err, aerr.StateFactDisputed) {
		t.Errorf("disputed fact used for a decision: %v", err)
	}
}

func TestNewFactSupersedesDisputed(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	if _, err := s.Verify(f.ID, checkAll(Mismatch), nil, ""); err != nil {
		t.Fatal(err)
	}
	next, created, err := s.Add(draft(t))
	if err != nil || !created || next.ID == f.ID || next.Supersedes == nil || *next.Supersedes != f.ID {
		t.Fatalf("replacement = %+v created=%v %v", next, created, err)
	}
	old, _, _ := s.Get(f.ID)
	if old.Status != Superseded {
		t.Errorf("old fact status = %s", old.Status)
	}
	if _, err := s.Verify(f.ID, checkAll(Match), nil, ""); !aerr.HasCode(err, aerr.StateFactDisputed) {
		t.Errorf("verify of a superseded fact: %v", err)
	}
	if v, _ := s.Find(Query{}); len(v) != 1 || v[0].ID != next.ID {
		t.Errorf("find hides superseded facts: %+v", v)
	}
	if v, _ := s.Find(Query{All: true}); len(v) != 2 {
		t.Errorf("find --all = %d facts", len(v))
	}
}

func TestDisputedFactCanBeReverified(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	if _, err := s.Verify(f.ID, checkAll(Mismatch), nil, ""); err != nil {
		t.Fatal(err)
	}
	res, err := s.Verify(f.ID, checkAll(Match), nil, "")
	if err != nil || res.Fact.Status != Active || res.Result != ResultVerified {
		t.Errorf("re-verified = %+v %v", res, err)
	}
}

func TestModelTextNotTrusted(t *testing.T) {
	// A step judged from tool output only ever reads the captured observation: there is no way
	// to hand Verify the text the model says it saw.
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t, func(d *Draft) {
		d.Verify = []Step{{"tool": "kubectl", "action": "get", "pattern": "crm-sync"}}
	}))
	r := observed(map[string]observe.Observation{"toolu_real": {Tool: "Bash", Input: `{"command":"kubectl get deploy"}`, Output: `{"stdout":"payments 1/1"}`}},
		map[int]string{0: "toolu_real"})
	res, err := s.Verify(f.ID, r.Check, nil, "")
	if err != nil || res.Result != ResultDisputed {
		t.Errorf("verify = %+v %v", res, err)
	}
}

func TestVerifyErrors(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Verify("F-1", checkAll(Match), nil, ""); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("bad id: %v", err)
	}
	if _, err := s.Verify("F-0001", checkAll(Match), &Ref{}, ""); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("bad ref: %v", err)
	}
	if _, err := s.Verify("F-0001", checkAll(Match), nil, ""); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("unknown fact: %v", err)
	}
	f := mustAdd(t, s, draft(t))
	if _, err := s.Verify(f.ID, checkAll(Match), &Ref{Change: "CHG-001"}, "bad session"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad session: %v", err)
	}
	bad := f
	bad.VerifiedAt = "yesterday"
	appendRaw(t, s, bad)
	if _, err := s.Verify(f.ID, checkAll(Unreachable), &Ref{Change: "CHG-001"}, "s"); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad verified_at: %v", err)
	}
	if err := os.MkdirAll(s.accessPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dependents(f.ID); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable access log: %v", err)
	}
	empty, _ := newStore(t)
	if err := os.MkdirAll(empty.factsPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.Verify("F-0001", checkAll(Match), nil, ""); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable cache: %v", err)
	}
}

func TestSupersedeAndDependentErrors(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	broken := f
	broken.Status, broken.Confidence = Disputed, "certain"
	appendRaw(t, s, broken)
	if _, _, err := s.Add(draft(t)); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("superseding an invalid record: %v", err)
	}
	if err := os.WriteFile(s.accessPath(), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dependents(f.ID); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt access log: %v", err)
	}
}

func TestWriteFailuresReported(t *testing.T) {
	s, _ := newStore(t)
	f := mustAdd(t, s, draft(t))
	bad := f
	bad.Confidence = "certain"
	appendRaw(t, s, bad)
	if _, err := s.Verify(f.ID, checkAll(Match), nil, ""); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("settling an invalid record: %v", err)
	}
	ro, _ := newStore(t)
	mustAdd(t, ro, draft(t))
	if err := os.Chmod(ro.factsPath(), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ro.Add(draft(t, func(d *Draft) { d.Env = "dev" })); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("append to a read-only cache: %v", err)
	}
}

func TestUsedFor(t *testing.T) {
	s, clk := newStore(t)
	f := mustAdd(t, s, draft(t))
	g := mustAdd(t, s, draft(t, func(d *Draft) { d.Env = "staging" }))
	if _, err := s.Use(f.ID, Orientation, Ref{Change: "CHG-001"}, "s"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(48 * time.Hour)
	if _, err := s.Use(g.ID, Decision, Ref{Change: "CHG-001"}, "s"); err == nil {
		t.Fatal("stale decision use allowed")
	}
	if _, err := s.Use(g.ID, Orientation, Ref{Ask: "ASK-001"}, "s"); err != nil {
		t.Fatal(err)
	}
	used, err := s.UsedFor(Ref{Change: "CHG-001"})
	if err != nil || !used[f.ID] || used[g.ID] || len(used) != 1 {
		t.Errorf("used for CHG-001 = %v %v (a refused use is not a use)", used, err)
	}
	if _, err := s.UsedFor(Ref{}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("no ref: %v", err)
	}
	if err := os.WriteFile(s.accessPath(), []byte("{\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UsedFor(Ref{Change: "CHG-001"}); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("corrupt access log: %v", err)
	}
	if err := os.Remove(s.accessPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.accessPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UsedFor(Ref{Change: "CHG-001"}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable access log: %v", err)
	}
}
