package m5set

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/tests/fixtures/shop/fakes"
	"github.com/mr-vaibh/athel/tests/fixtures/shop/systest"
)

const prd = "# Users profile v2\n\nOrders and the gateway read customer profiles from the users service's v2 endpoint.\n"

const review = `schema: athel/prd-review@1
change: CHG-001
version: v1
checklist:
  goal: {status: present, prd: [PRD-1]}
  actors: {status: missing, prd: []}
  scope: {status: missing, prd: []}
  out_of_scope: {status: missing, prd: []}
  functional_requirements: {status: present, prd: [PRD-1]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: missing, prd: []}
  constraints: {status: missing, prd: []}
  dependencies: {status: missing, prd: []}
ambiguities: []
contradictions: []
assumptions: []
`

// spec is one service's spec: one requirement and its acceptance criterion.
func spec(service, req, ac, text, given, when, then string) string {
	return fmt.Sprintf(`schema: athel/spec@1
change: CHG-001
service: %s
repo: github.com/acme/%s
requirements:
  - id: %s
    prd: [PRD-1]
    text: %s
    decisions: []
interfaces_changed: []
data_changes: []
design_notes: []
acceptance_criteria:
  - {id: %s, req: [%s], given: %s, when: %s, then: %s}
out_of_scope: []
assumptions: []
`, service, service, req, text, ac, req, given, when, then)
}

const blastReport = `schema: athel/blast@1
change: CHG-001
mode: planned
proposal: Users serves profiles on /v2; orders and the gateway move to it; v1 is removed later
services_changed: [users, orders, gateway]
direct:
  - {item: "GET /v2/users/{id}", kind: interface, change: additive, service: users, evidence: [{kind: spec, ref: REQ-1}]}
consumers:
  - {consumer: orders, of: "GET /v2/users/{id}", env: prod, in_change_set: true, traffic: null, evidence: [{kind: spec, ref: REQ-2}], facts: []}
  - {consumer: gateway, of: "GET /v2/users/{id}", env: prod, in_change_set: true, traffic: null, evidence: [{kind: spec, ref: REQ-3}], facts: []}
data: []
infra: []
people: []
runtime: []
flags: {touches_auth_payments_pii: false, security_review_task: false, retry_timeout_or_idempotency_change: false}
not_affected: [{item: payments, why: never reads customer profiles, evidence: [{kind: code, ref: payments/}]}]
gaps: []
risk: LOW
rules_fired: []
`

const planDoc = `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: Serve profiles on /v2, repo: users, kind: expand, paths: ["api/**"], verbs: [edit, create, test, commit, push], covers: [AC-1], depends_on: [], size: S, gates: [], rollback: revert the commit}
  - {id: T-02, title: Read customers from /v2, repo: orders, kind: consume, paths: ["clients/**", "api/**"], verbs: [edit, test, commit, push], covers: [AC-2], depends_on: [T-01], size: S, gates: [], rollback: revert the commit}
  - {id: T-03, title: Route /profile/ to users, repo: gateway, kind: consume, paths: [main.go, main_test.go, routes.yaml], verbs: [edit, test, commit, push], covers: [AC-3], depends_on: [T-01], size: S, gates: [], rollback: revert the commit}
  - {id: T-04, title: "Remove GET /v1/users/{id}", repo: users, kind: contract, deferred: true, paths: ["api/**"], verbs: [edit, delete, test, commit, push], covers: [AC-1], depends_on: [T-02, T-03], size: S, gates: [], rollback: restore the route}
`

// specify is the start, spec and plan steps up to the developer's approval.
func (j *journey) specify() {
	j.t.Helper()
	if _, err := j.sb.Athel("", "--json", "change", "new", "--title", "Users profile v2", "--prd-text", prd); err != nil {
		j.t.Fatal(err)
	}
	j.to("investigating")
	j.athel(review, "prd", "review", "save", change, "-")
	for _, r := range []string{"users", "orders", "gateway"} {
		j.athel("", "repos", "cache", r)
		j.athel("", "repos", "survey", r)
		j.athel("conventions: |\n  # Conventions\n  Go 1.27, table-driven tests.\nmap: |\n  # Map\n  main.go wires the handlers.\n",
			"conventions", "save", "github.com/acme/"+r, "-")
	}
	j.athel(spec("users", "REQ-1", "AC-1", "The users service shall serve a customer's profile at GET /v2/users/{id}.",
		"a customer c-1", "GET /v2/users/c-1 is called", "the answer is 200 with id c-1"), "spec", "save", change, "-")
	j.athel(spec("orders", "REQ-2", "AC-2", "When orders reads an order's customer, the orders service shall call the users service's v2 endpoint.",
		"order 7", "its customer is read", "users is called at /v2/users/c-7"), "spec", "save", change, "-")
	j.athel(spec("gateway", "REQ-3", "AC-3", "The gateway shall route /profile/ to the users service.",
		"the gateway", "GET /profile/v2/users/c-1 is called", "the answer is 200 with id c-1 from the users service"), "spec", "save", change, "-")
	j.athel(blastReport, "blast", "save", "--change", change, "-")
	j.to("specified")
	j.to("awaiting_decisions")
	j.athel(planDoc, "plan", "set", change, "-")
	j.athel("", "trace", change)
	j.to("planned")
	sum := j.athel("", "plan", "summary", change)
	if order := fmt.Sprint(sum["merge_order"]); !strings.Contains(order, "1. users (provider)") || !strings.Contains(order, "2. gateway, orders (consumer; after users; either order)") {
		j.t.Fatalf("merge order in the summary: %v", sum["merge_order"])
	}
	j.prompt("/athel:approve " + change)
	j.to("plan_approved")
}

// build preflights every repo, answers the teammate-overlap card as the developer would, and
// builds the three tasks; the deferred removal is never offered.
func (j *journey) build() {
	j.t.Helper()
	for _, r := range []string{"users", "orders", "gateway"} {
		j.athel("", "git", "preflight", change, r)
	}
	if list := j.athel("", "decide", "list", change, "--open"); fmt.Sprint(list["decisions"]) != "[]" {
		var answers []string
		for _, d := range list["decisions"].([]any) {
			m := d.(map[string]any)
			answers = append(answers, fmt.Sprintf("%v=%v", m["id"], m["recommendation"].(map[string]any)["option"]))
		}
		line := "/athel:decide " + change + " " + strings.Join(answers, " ")
		j.prompt(line)
		j.athel("", append([]string{"decide", "answer", change}, answers...)...)
	}
	j.to("building")
	j.task("T-01", "users", "feat(users): serve profiles on /v2", "api/v2_test.go", map[string]string{
		"api/user.go":    usersV2(j),
		"api/v2_test.go": "package api\n\nimport (\n\t\"net/http\"\n\t\"strings\"\n\t\"testing\"\n)\n\n// TestGetUserV2 checks AC-1.\nfunc TestGetUserV2(t *testing.T) {\n\trec := serve(t, http.MethodGet, \"/v2/users/c-1\", \"\")\n\tif rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `\"id\":\"c-1\"`) {\n\t\tt.Fatalf(\"got %d %s\", rec.Code, rec.Body.String())\n\t}\n}\n",
	})
	j.athel("", "state", "enter", change, "build", "--task", "T-02")
	ctx := j.athel("", "build", "context", change, "T-02")
	if up := fmt.Sprint(ctx["upstream"]); !strings.Contains(up, "build/T-02/upstream-T-01.patch") {
		j.t.Fatalf("the consumer's manifest carries the provider's patch: %v", ctx["upstream"])
	}
	j.athel("", "state", "exit", change, "build", "--result", "ok")
	j.task("T-02", "orders", "feat(orders): read customers from users v2", "clients/clients_test.go", ordersV2(j))
	j.task("T-03", "gateway", "feat(gateway): route /profile/ to users", "main_test.go", gatewayProfile(j))
	next := j.athel("", "build", "next", change)
	if next["next"] != "" || fmt.Sprint(next["done"]) != "[T-01 T-02 T-03]" {
		j.t.Fatalf("every task but the deferred removal is done: %v", next)
	}
}

// ship pushes and opens the three PRs, rehearses the merge order with the system check, and
// marks the PRs ready together.
func (j *journey) ship() {
	j.t.Helper()
	repos := []string{"users", "orders", "gateway"}
	for _, r := range repos {
		j.athel("", "git", "push", change, r)
		j.athel("", "pr", "open", change, r)
	}
	j.to("ready_for_pr")
	if _, err := j.try("", "pr", "ready", change, "users"); err == nil || !strings.Contains(err.Error(), "has not been rehearsed") {
		j.t.Fatalf("pr ready before the rehearsal: %v", err)
	}
	cmd, _ := json.Marshal(systest.Command) // strings always encode
	if err := os.WriteFile(filepath.Join(j.sb.WS, ".athel", "config", "system.yaml"), []byte("schema: athel/system-check@1\ncommand: "+string(cmd)+"\n"), 0o600); err != nil {
		j.t.Fatal(err)
	}
	v := j.athel("", "set", "verify", change)
	if v["result"] != "pass" || len(v["steps"].([]any)) != 4 {
		b, _ := json.MarshalIndent(v, "", " ") // a plain map always encodes
		j.t.Fatalf("rehearsal:\n%s", b)
	}
	for _, r := range repos {
		j.athel("", "pr", "update", change, r)
		j.athel("", "pr", "ready", change, r)
	}
	j.to("in_review")
	body := j.prBody("acme/orders", 902)
	for _, want := range []string{"This PR is step 2 of 2 in the merge order of CHG-001 (3 PRs). Merge after acme/users#901.",
		"1. acme/users#901 (provider)", "Later, a separate change: users", "Merge-order rehearsal: pass at"} {
		if !strings.Contains(body, want) {
			j.t.Errorf("the orders PR body lacks %q:\n%s", want, body)
		}
	}
}

// merge is the human: PRs merged in the stated order, the system checked from main after each
// merge, status read between them, then the follow-up drafted.
func (j *journey) merge() {
	j.t.Helper()
	gh := fakes.Shim{State: j.sb.GHState, Out: io.Discard, Err: io.Discard}
	status := j.athel("", "set", "status", change)
	if fmt.Sprint(status["next"]) != "[acme/users#901]" || fmt.Sprint(status["alarms"]) != "[]" {
		j.t.Fatalf("before any merge: %v", status)
	}
	order := []struct {
		repo string
		pr   int
	}{{"users", 901}, {"orders", 902}, {"gateway", 903}}
	for i, m := range order {
		if err := gh.Merge("acme/"+m.repo, m.pr, j.sb.Layout.Repos[m.repo].Bare, fmt.Sprintf("2026-11-25T1%d:00:00Z", i)); err != nil {
			j.t.Fatal(err)
		}
		if failed := j.systemFromMain(); failed != "" {
			j.t.Fatalf("after merging %s, main is broken: %s", m.repo, failed)
		}
		if s := j.athel("", "set", "status", change); fmt.Sprint(s["alarms"]) != "[]" {
			j.t.Fatalf("after merging %s in order: %v", m.repo, s["alarms"])
		}
	}
	status = j.athel("", "set", "status", change)
	if !strings.Contains(fmt.Sprint(status["cleanup"]), "run /athel:followup") {
		j.t.Fatalf("after every merge: %v", status)
	}
	j.prompt("/athel:followup " + change)
	f := j.athel("", "followup", "draft", change)
	b, err := os.ReadFile(f["path"].(string))
	if err != nil || !strings.Contains(string(b), "Remove GET /v1/users/{id}, in users") {
		j.t.Fatalf("follow-up: %v\n%s", err, b)
	}
}

func (j *journey) prBody(repo string, n int) string {
	j.t.Helper()
	b, err := os.ReadFile(filepath.Join(j.sb.GHState, "prs.json"))
	if err != nil {
		j.t.Fatal(err)
	}
	var prs []struct {
		Repo   string `json:"repo"`
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(b, &prs); err != nil {
		j.t.Fatal(err)
	}
	for _, p := range prs {
		if p.Repo == repo && p.Number == n {
			return p.Body
		}
	}
	j.t.Fatalf("no PR %s#%d", repo, n)
	return ""
}

// systemFromMain clones main of every service from the bare remotes and runs the system check;
// it returns the failed flows, "" when every flow passes.
func (j *journey) systemFromMain() string {
	j.t.Helper()
	dirs := map[string]string{}
	for _, r := range shopServices {
		d := filepath.Join(j.t.TempDir(), r)
		if out, err := exec.Command("git", "clone", "-q", j.sb.Layout.Repos[r].Bare, d).CombinedOutput(); err != nil {
			j.t.Fatalf("clone %s: %v %s", r, err, out)
		}
		dirs[r] = d
	}
	results, err := systest.Run(context.Background(), dirs)
	if err != nil {
		j.t.Fatal(err)
	}
	return strings.Join(systest.Failed(results), "; ")
}

var shopServices = []string{"users", "orders", "gateway"}

// The developer's code for each task: the shop's own files with the change.

func read(j *journey, repo, name string) string {
	j.t.Helper()
	b, err := os.ReadFile(filepath.Join(j.sb.WS, "repos", repo+"@"+change, name))
	if err != nil {
		j.t.Fatal(err)
	}
	return string(b)
}

func usersV2(j *journey) string {
	src := read(j, "users", "api/user.go")
	return strings.Replace(src, "\tmux.HandleFunc(\"GET /v1/users/{id}\", getUser)\n",
		"\tmux.HandleFunc(\"GET /v1/users/{id}\", getUser)\n\tmux.HandleFunc(\"GET /v2/users/{id}\", getUser)\n", 1)
}

func ordersV2(j *journey) map[string]string {
	return map[string]string{
		"clients/users.go": strings.Replace(strings.Replace(read(j, "orders", "clients/users.go"),
			"\treturn http.Get(u.Base + \"/v1/users/\" + id) //nolint:gosec,noctx // fixture code, never run\n",
			"\treq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.Base+\"/v2/users/\"+id, nil)\n"+
				"\tif err != nil {\n\t\treturn nil, err\n\t}\n\treturn http.DefaultClient.Do(req)\n", 1),
			"import \"net/http\"", "import (\n\t\"context\"\n\t\"net/http\"\n)", 1),
		"clients/clients_test.go": strings.Replace(read(j, "orders", "clients/clients_test.go"), `"GET /v1/users/u-1 ", "GET /v1/users/u-1/legacy-profile "`, `"GET /v2/users/u-1 ", "GET /v1/users/u-1/legacy-profile "`, 1) +
			"\n// TestUsersBadBase checks AC-2's error path: a base that is no URL is an error, not a call.\n" +
			"func TestUsersBadBase(t *testing.T) {\n\tif _, err := (Users{Base: \"://bad\"}).Get(\"u-1\"); err == nil {\n\t\tt.Fatal(\"no error\")\n\t}\n}\n",
		"api/customer_test.go": strings.Replace(strings.Replace(read(j, "orders", "api/customer_test.go"),
			`r.URL.Path != "/v1/users/c-7"`, `r.URL.Path != "/v2/users/c-7"`, 1),
			"func TestCustomer(t *testing.T) {", "// TestCustomer checks AC-2: orders reads its customer from users v2.\nfunc TestCustomer(t *testing.T) {", 1),
	}
}

func gatewayProfile(j *journey) map[string]string {
	return map[string]string{
		"main.go": strings.Replace(read(j, "gateway", "main.go"), `"/users/": "http://user-svc:8080"}`,
			`"/users/": "http://user-svc:8080", "/profile/": "http://user-svc:8080"}`, 1),
		"routes.yaml": read(j, "gateway", "routes.yaml") + "  - {prefix: /profile/, service: user-svc, port: 8080}\n",
		"main_test.go": read(j, "gateway", "main_test.go") + "\n// TestProfileRoute checks AC-3: /profile/ reaches the users service.\n" +
			"func TestProfileRoute(t *testing.T) {\n\tif routes[\"/profile/\"] != \"http://user-svc:8080\" {\n\t\tt.Fatal(routes)\n\t}\n}\n",
	}
}
