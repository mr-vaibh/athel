package engine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/config"
	"github.com/mr-vaibh/athel/cli/internal/envdetect"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const (
	ws   = "/ws"
	tree = "/ws/repos/svc@CHG-001"
)

const envYAML = `schema: athel/environments@1
environments:
  prod:
    kube_contexts: ["gke_acme-prod_*"]
  staging:
    kube_contexts: ["gke_acme-staging_*"]
  dev:
    kube_contexts: ["kind-*"]
default: prod
`

const toolsYAML = `schema: athel/tools@1
tools:
  - id: helm
    kind: cli
    match: {command: helm}
    purpose: [deployed_state]
    read: [list, status, get]
    write: [repo add]
    policy:
      dev: {read: allow, write: allow}
      staging: {read: approve, write: deny}
  - id: jira
    kind: mcp
    match: {server: jira}
    purpose: [tickets]
    read: [get_issue]
    write: [create_issue]
`

func policySet(t *testing.T) *policy.Set {
	t.Helper()
	proj := t.TempDir()
	for name, body := range map[string]string{"environments.yaml": envYAML, "tools.yaml": toolsYAML} {
		if err := os.WriteFile(filepath.Join(proj, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pol, err := policy.Load(config.Paths{Home: t.TempDir(), ProjectDir: proj}, schema.NewRegistry(), "sha256:x")
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

func approvedPlan() *plan.Plan {
	return &plan.Plan{Schema: "athel/plan@1", Change: "CHG-001", Tasks: []plan.Task{
		{ID: "T1", Repo: "svc", Paths: []string{"src/**"}, Verbs: []string{"edit", "create", "delete", "commit", "test", "push"}},
		{ID: "T2", Repo: "svc", Paths: []string{"docs/**"}, Verbs: []string{"edit"}, EnvActions: []string{"jira"}},
	}}
}

func changeCtx(t *testing.T) Context {
	t.Helper()
	pol := policySet(t)
	files := map[string]string{"/home/dev/.kube/config": "current-context: kind-dev\n"}
	return Context{Mode: Change, ChangeID: "CHG-001", Plan: approvedPlan(), WorkspaceRoot: ws,
		OwnBranches: []string{"athel/CHG-001-svc"}, Policy: pol,
		Resolver: &envdetect.Resolver{Policy: pol, Home: "/home/dev",
			Getenv: func(string) string { return "" },
			ReadFile: func(p string) ([]byte, error) {
				if s, ok := files[p]; ok {
					return []byte(s), nil
				}
				return nil, errors.New("not found")
			}}}
}

func bash(cmd string) (string, map[string]any) {
	return "Bash", map[string]any{"command": cmd}
}

func decide(t *testing.T, ctx Context, tool string, input any) Result {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	call := classify.Call{Tool: tool, Input: raw, Cwd: tree}
	cr := classify.Classify(call, ctx.Policy, classify.Options{ReadFile: func(p string) ([]byte, error) {
		if strings.HasSuffix(p, ".git/HEAD") {
			return []byte("ref: refs/heads/athel/CHG-001-svc\n"), nil
		}
		return nil, errors.New("not found")
	}})
	return Decide(call, cr, ctx)
}

type ruleCase struct {
	name   string
	tool   string
	input  any
	modify func(*Context)
	want   Decision
	rule   string
	code   aerr.Code
}

func TestRuleOrder(t *testing.T) {
	file := func(p string) map[string]any { return map[string]any{"file_path": p} }
	edit := func(p string) map[string]any {
		return map[string]any{"file_path": p, "old_string": "a", "new_string": "b"}
	}
	b := func(cmd string) map[string]any { _, in := bash(cmd); return in }
	cases := []ruleCase{
		{"0 invalid policy denies writes", "Edit", edit(tree + "/src/a.go"), func(c *Context) { c.PolicyErr = errors.New("bad") }, Deny, "0", aerr.PolicyConfigInvalid},
		{"0 invalid policy keeps reads", "Read", file(tree + "/src/a.go"), func(c *Context) { c.PolicyErr = errors.New("bad") }, Allow, "12", ""},
		{"1 malformed", "Edit", map[string]any{"file_path": 3}, nil, Deny, "1", aerr.PolicyMalformed},
		{"2 unparseable write", "Bash", b("echo $(( 1 +"), nil, Deny, "2", aerr.PolicyUnparseable},
		{"3 forbidden", "Bash", b("git push --force origin athel/CHG-001-svc"), nil, Deny, "3", aerr.PolicyForbidden},
		{"5 raptor without an environment is prod", "Bash", b("raptor create resource x"), nil, Deny, "5", aerr.PolicyProdWrite},
		{"5 prod write", "Bash", b("kubectl --context gke_acme-prod_eu scale deploy x --replicas=2"), nil, Deny, "5", aerr.PolicyProdWrite},
		{"5 unknown env is prod", "Bash", b("kubectl --context mystery scale deploy x --replicas=2"), nil, Deny, "5", aerr.PolicyProdWrite},
		{"6 irreversible in dev", "Bash", b("kubectl --context kind-dev delete ns x"), nil, Deny, "6", aerr.PolicyIrreversible},
		{"7 no plan", "Edit", edit(tree + "/src/a.go"), func(c *Context) { c.Plan = nil }, Deny, "7", aerr.PolicyScope},
		{"7 path outside plan", "Edit", edit(tree + "/main.go"), nil, Deny, "7", aerr.PolicyScope},
		{"7 verb outside plan", "Write", file(tree + "/docs/new.md"), nil, Deny, "7", aerr.PolicyScope},
		{"7 other change worktree", "Edit", edit("/ws/repos/svc@CHG-002/src/a.go"), nil, Deny, "7", aerr.PolicyScope},
		{"7 outside workspace", "Edit", edit("/etc/hosts"), nil, Deny, "7", aerr.PolicyScope},
		{"7 shared write not planned", "Bash", b("gh pr create --title x --body y"), nil, Deny, "7", aerr.PolicyScope},
		{"8 planned shared write needs a human", "mcp__jira__create_issue", map[string]any{"summary": "x"}, nil, Approve, "8", ""},
		{"7 unplanned shared write", "mcp__jira__create_issue", map[string]any{"summary": "x"}, func(c *Context) { c.Plan.Tasks[1].EnvActions = nil }, Deny, "7", aerr.PolicyScope},
		{"11 registered MCP read", "mcp__jira__get_issue", map[string]any{"id": "x"}, nil, Allow, "11", ""},
		{"9 own branch", "Bash", b("git push origin athel/CHG-001-svc"), nil, Allow, "9", ""},
		{"9 foreign branch", "Bash", b("git push origin feature/x"), func(c *Context) {
			c.Plan.Tasks[0].Verbs = append(c.Plan.Tasks[0].Verbs, "push")
		}, Deny, "9", aerr.PolicyForeignBranch},
		{"10 worktree edit", "Edit", edit(tree + "/src/a.go"), nil, Allow, "10", ""},
		{"10 worktree commit", "Bash", b("git commit -m x"), nil, Allow, "10", ""},
		{"11 registered remote read", "Bash", b("kubectl --context kind-dev get pods"), nil, Allow, "11", ""},
		{"11 unregistered remote read", "WebFetch", map[string]any{"url": "https://example.com", "prompt": "x"}, nil, Approve, "11", ""},
		{"12 read inside", "Read", file(tree + "/src/a.go"), nil, Allow, "12", ""},
		{"12 read outside", "Read", file("/usr/share/dict/words"), nil, Approve, "12", ""},
		{"12 secret path", "Read", file("/home/dev/.ssh/id_ed25519"), nil, Deny, "12", aerr.PolicySecretPath},
		{"7x athel CLI", "Bash", b("athel state transition CHG-001 building"), func(c *Context) { c.Plan = nil }, Allow, "7x", ""},
		{"7u unknown command", "Bash", b("frobnicate --all"), nil, Approve, "7u", ""},
	}
	run(t, cases)
}

func run(t *testing.T, cases []ruleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := changeCtx(t)
			if tc.modify != nil {
				tc.modify(&ctx)
			}
			got := decide(t, ctx, tc.tool, tc.input)
			if got.Decision != tc.want || got.Rule != tc.rule || got.Code != tc.code {
				t.Fatalf("got %s rule %s code %q (%s), want %s rule %s code %q", got.Decision, got.Rule, got.Code, got.Reason, tc.want, tc.rule, tc.code)
			}
		})
	}
}

func TestGuardMode(t *testing.T) {
	guard := func(c *Context) { c.Mode, c.Plan, c.ChangeID = Guard, nil, "" }
	file := map[string]any{"file_path": "/anywhere/a.go", "old_string": "a", "new_string": "b"}
	_, rmProd := bash("kubectl --context gke_acme-prod_eu delete pod x")
	_, unknown := bash("frobnicate --all")
	_, unparse := bash("cat $(( 1 +")
	run(t, []ruleCase{
		{"edits outside a change get no decision", "Edit", file, guard, None, "-", ""},
		{"unknown commands get no decision", "Bash", unknown, guard, None, "-", ""},
		{"prod writes are still denied", "Bash", rmProd, guard, Deny, "5", aerr.PolicyProdWrite},
		{"unparseable commands are denied", "Bash", unparse, guard, Deny, "2", aerr.PolicyUnparseable},
	})
}

func TestUnparseableReadNeedsApproval(t *testing.T) {
	cr := classify.Result{Unparseable: true, Reasons: []string{"x"}, Actions: []classify.Action{{Tool: "cat", Tier: classify.R0, Text: "cat a"}}}
	for _, mode := range []Mode{Guard, Change} {
		ctx := changeCtx(t)
		ctx.Mode = mode
		if got := Decide(classify.Call{Tool: "Bash"}, cr, ctx); got.Decision != Approve || got.Rule != "2" {
			t.Fatalf("%s: got %s rule %s", mode, got.Decision, got.Rule)
		}
	}
}

func TestStrictestWins(t *testing.T) {
	cases := []struct {
		cmd  string
		want Decision
		rule string
	}{
		{"ls src && cat src/a.go", Allow, "12"},
		{"ls src && cat /usr/share/dict/words", Approve, "12"},
		{"cat /usr/share/dict/words; git push --force origin x", Deny, "3"},
		{"cat src/a.go | kubectl --context gke_acme-prod_eu apply -f -", Deny, "5"},
		{"ls src && rm -rf /etc/x", Deny, "7"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			name, in := bash(tc.cmd)
			got := decide(t, changeCtx(t), name, in)
			if got.Decision != tc.want || got.Rule != tc.rule {
				t.Fatalf("%q: got %s rule %s (%s), want %s rule %s", tc.cmd, got.Decision, got.Rule, got.Reason, tc.want, tc.rule)
			}
			if len(got.Actions) < 2 {
				t.Fatalf("want every action decided, got %d", len(got.Actions))
			}
		})
	}
}

func TestAthelStateExceptionRule7(t *testing.T) {
	ctx := changeCtx(t)
	ctx.Plan = nil
	name, in := bash("athel plan set CHG-001 plan.yaml")
	if got := decide(t, ctx, name, in); got.Decision != Allow || got.Rule != "7x" {
		t.Fatalf("athel CLI: got %s rule %s", got.Decision, got.Rule)
	}
	direct := map[string]any{"file_path": ws + "/.athel/changes/CHG-001/change.yaml", "content": "x"}
	if got := decide(t, ctx, "Write", direct); got.Decision != Deny || got.Rule != "3" {
		t.Fatalf("direct state write: got %s rule %s (%s)", got.Decision, got.Rule, got.Reason)
	}
	name, in = bash("echo x > " + ws + "/.athel/changes/CHG-001/change.yaml")
	if got := decide(t, ctx, name, in); got.Decision != Deny {
		t.Fatalf("redirect into state: got %s", got.Decision)
	}
}

func TestReasonHasAllFields(t *testing.T) {
	ctx := changeCtx(t)
	ctx.Agent = "athel-builder"
	got := decide(t, ctx, "WebFetch", map[string]any{"url": "https://example.com/x", "prompt": "x"})
	for _, want := range []string{"command: fetch https://example.com/x", "environment: ", "plan task: none", "reason: ", "source: proposed by the subagent athel-builder", "rule 11"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("reason %q lacks %q", got.Reason, want)
		}
	}
	ctx.Agent = ""
	got = decide(t, ctx, "Read", map[string]any{"file_path": "/usr/share/dict/words"})
	if !strings.Contains(got.Reason, "source: proposed by the main agent") {
		t.Errorf("main agent source missing: %q", got.Reason)
	}
	got = decide(t, ctx, "Edit", map[string]any{"file_path": "/etc/hosts", "old_string": "a", "new_string": "b"})
	for _, want := range []string{"Athel denied this", string(aerr.PolicyScope), "rule 7", "CHG-001"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("deny reason %q lacks %q", got.Reason, want)
		}
	}
	got = decide(t, ctx, "Read", map[string]any{"file_path": tree + "/src/a.go"})
	if !strings.HasPrefix(got.Reason, "Athel allowed") {
		t.Errorf("allow reason: %q", got.Reason)
	}
}

func TestPlannedTaskIsReported(t *testing.T) {
	got := decide(t, changeCtx(t), "Edit", map[string]any{"file_path": tree + "/src/a.go", "old_string": "a", "new_string": "b"})
	if got.Actions[0].Task != "T1" || got.Tier != classify.W0 {
		t.Fatalf("task %q tier %s", got.Actions[0].Task, got.Tier)
	}
}

func TestHeadlessApproveBecomesDeny(t *testing.T) {
	ctx := changeCtx(t)
	ctx.Headless = true
	got := decide(t, ctx, "Read", map[string]any{"file_path": "/usr/share/dict/words"})
	if got.Decision != Deny || got.Code != aerr.PolicyHeadless || got.Rule != "12+headless" {
		t.Fatalf("got %s %s rule %s", got.Decision, got.Code, got.Rule)
	}
	if !strings.Contains(got.Reason, "headless") {
		t.Fatalf("reason %q", got.Reason)
	}
	got = decide(t, ctx, "Read", map[string]any{"file_path": tree + "/src/a.go"})
	if got.Decision != Allow {
		t.Fatalf("headless allow changed to %s", got.Decision)
	}
}

func TestToolPolicySettings(t *testing.T) {
	ctx := changeCtx(t)
	set := func(env string) func(*Context) {
		return func(c *Context) {
			c.Resolver.ReadFile = func(string) ([]byte, error) {
				return []byte("current-context: " + env + "\n"), nil
			}
		}
	}
	_, read := bash("helm status x")
	run(t, []ruleCase{
		{"staging read needs approval", "Bash", read, set("gke_acme-staging_eu"), Approve, "11", ""},
		{"prod read without a rule is allowed", "Bash", read, set("gke_acme-prod_eu"), Allow, "11", ""},
	})
	verdict := func(dec Decision, rule string, code aerr.Code, why string) ActionDecision {
		return ActionDecision{Decision: dec, Rule: rule, Code: code, Why: why}
	}
	a := classify.Action{Tool: "helm", ToolID: "helm", Tier: classify.W2}
	for env, want := range map[string]Decision{"dev": Allow, "staging": Deny, "n/a": Approve} {
		if d := ctx.toolPolicy(&ActionDecision{Env: env}, a, "write", verdict, "8"); d.Decision != want {
			t.Errorf("helm write in %s: %s, want %s", env, d.Decision, want)
		}
	}
	ctx.Policy = nil
	if d := ctx.toolPolicy(&ActionDecision{Env: "dev"}, a, "write", verdict, "8"); d.Decision != Approve {
		t.Fatalf("without policy: %s", d.Decision)
	}
}

func TestPlanTaskEdges(t *testing.T) {
	ctx := changeCtx(t)
	if task, why := ctx.planTask(classify.Action{Tier: classify.W0, Text: "x"}, ""); task != nil || !strings.Contains(why, "no target") {
		t.Fatalf("no paths: %v %q", task, why)
	}
	if _, _, ok := ctx.worktreeOf(tree + "/../svc@CHG-001/x"); ok {
		t.Fatal("dotdot accepted")
	}
	if _, _, ok := ctx.worktreeOf("/ws/repos/svc/x"); ok {
		t.Fatal("worktree without change id accepted")
	}
	ctx.WorkspaceRoot = ""
	if ctx.inWorkspace("/ws/a") || ctx.inWorktree(tree+"/a") {
		t.Fatal("empty workspace root matched")
	}
	if (Context{WorkspaceRoot: ws}).inWorkspace("$HOME/x") {
		t.Fatal("$HOME matched")
	}
}

func TestVerbOf(t *testing.T) {
	cases := map[string]classify.Action{
		"push":    {Tier: classify.W1, Tool: "git"},
		"create":  {Tier: classify.W0, Tool: "Write"},
		"delete":  {Tier: classify.W0, Tool: "rm"},
		"commit":  {Tier: classify.W0, Tool: "git"},
		"test":    {Tier: classify.W0, Tool: "go"},
		"edit":    {Tier: classify.W0, Tool: "Edit"},
		"pr":      {Tier: classify.W1, Tool: "gh", Verb: "pr create"},
		"comment": {Tier: classify.W2, Tool: "gh", Verb: "issue comment"},
	}
	for want, a := range cases {
		if got := verbOf(a); got != want {
			t.Errorf("verbOf(%s) = %s, want %s", a.Tool, got, want)
		}
	}
}

func TestDefaultRuleAndNoDecisionReason(t *testing.T) {
	ctx := changeCtx(t)
	verdict := func(dec Decision, rule string, code aerr.Code, why string) ActionDecision {
		return ActionDecision{Decision: dec, Rule: rule, Code: code, Why: why}
	}
	if d := ctx.changeRules(&ActionDecision{}, classify.Action{Tier: "Z"}, "", verdict); d.Rule != "13" || d.Code != aerr.PolicyDefault {
		t.Fatalf("rule 13: %+v", d)
	}
	if r := ctx.reason(Result{Decision: Deny, Rule: "3"}); r != "Athel: no decision" {
		t.Fatalf("reason without actions: %q", r)
	}
	res := Result{Decision: None, Rule: "-", Actions: []ActionDecision{{Decision: None, Rule: "-", Why: "w"}}}
	if r := ctx.reason(res); r != "Athel: w" {
		t.Fatalf("none reason: %q", r)
	}
}

func TestHumanOriginRule(t *testing.T) {
	transition := "athel state transition CHG-001 --to plan_approved --by human --reason ok"
	granted := func(c *Context) {
		c.HumanGrant = func(change, target string) bool { return change == "CHG-001" && target == "plan_approved" }
	}
	_, in := bash(transition)
	_, other := bash("athel state transition CHG-002 --to plan_approved --by human --reason ok")
	_, byAthel := bash("athel state transition CHG-001 --to investigating --by athel --reason ok")
	_, acceptEdit := bash("athel integrity accept CHG-001 --reason fine")
	run(t, []ruleCase{
		{"no grant", "Bash", in, nil, Deny, "H", aerr.PolicyHumanOrigin},
		{"grant for this change", "Bash", in, granted, Allow, "7x", ""},
		{"grant for another change", "Bash", other, granted, Deny, "H", aerr.PolicyHumanOrigin},
		{"subagent with a grant", "Bash", in, func(c *Context) { granted(c); c.Subagent, c.Agent = true, "athel:developer" }, Deny, "H", aerr.PolicyHumanOrigin},
		{"guard mode with a grant", "Bash", in, func(c *Context) { granted(c); c.Mode = Guard }, None, "-", ""},
		{"by athel needs no grant", "Bash", byAthel, nil, Allow, "7x", ""},
		{"integrity accept is the developer's", "Bash", acceptEdit, nil, Deny, "H", aerr.PolicyHumanOrigin},
	})
}
