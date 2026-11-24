package plan

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const valid = `schema: athel/plan@1
change: CHG-042
hash: null
tasks:
  - id: T-02
    title: Add column
    repo: user-service
    paths: ["db/migrations/**"]
    verbs: [create, edit, test, commit]
    covers: [AC-3]
    depends_on: []
    size: S
    gates: [build, migration]
  - id: T-03
    title: Add PUT /v1/users/{id}/phone
    repo: user-service
    paths: ["src/profile/**", "src/validation/phone.ts", "test/profile/**"]
    verbs: [edit, create, test, commit, push]
    covers: [AC-1, AC-2]
    depends_on: [T-02]
    size: M
    gates: [build, unit, lint]
    risk_notes: "Hot path"
    rollback: "Feature flag off"
    env_actions: []
`

func TestPlanSchema(t *testing.T) {
	p, err := Decode(schema.NewRegistry(), []byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 2 || p.Tasks[1].DependsOn[0] != "T-02" || p.Change != "CHG-042" {
		t.Errorf("plan = %+v", p)
	}
	bad := []struct {
		name, from, to string
		code           aerr.Code
	}{
		{"unknown verb", "verbs: [create, edit, test, commit]", "verbs: [deploy]", aerr.StateSchema},
		{"bad task id", "id: T-02", "id: T2", aerr.StateSchema},
		{"unknown field", "size: S", "size: S\n    owner: x", aerr.StateSchema},
		{"duplicate id", "id: T-03", "id: T-02", aerr.StateInvalid},
		{"unknown dependency", "depends_on: [T-02]", "depends_on: [T-09]", aerr.StateInvalid},
		{"cycle", "depends_on: []", "depends_on: [T-03]", aerr.StateInvalid},
		{"self cycle", "depends_on: [T-02]", "depends_on: [T-03]", aerr.StateInvalid},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			doc := strings.Replace(valid, tt.from, tt.to, 1)
			if _, err := Decode(schema.NewRegistry(), []byte(doc)); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestHashStableAndBinding(t *testing.T) {
	reg := schema.NewRegistry()
	p, _ := Decode(reg, []byte(valid))
	h1 := ComputeHash(p)
	out, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Decode(reg, out)
	if err != nil {
		t.Fatal(err)
	}
	if *again.Hash != h1 || ComputeHash(again) != h1 {
		t.Errorf("hash not stable across encode: %s vs %s", *again.Hash, h1)
	}
	again.Tasks[0].Paths = append(again.Tasks[0].Paths, "src/**")
	if ComputeHash(again) == h1 {
		t.Error("changing a task must change the hash")
	}
}

func TestTaskMatch(t *testing.T) {
	p, _ := Decode(schema.NewRegistry(), []byte(valid))
	tests := []struct {
		w    Write
		want string
	}{
		{Write{Repo: "user-service", Path: "src/profile/handler.ts", Verb: "edit"}, "T-03"},
		{Write{Repo: "user-service", Path: "src/profile/deep/x/y.ts", Verb: "create"}, "T-03"},
		{Write{Repo: "user-service", Path: "src/validation/phone.ts", Verb: "edit"}, "T-03"},
		{Write{Repo: "user-service", Path: "db/migrations/001.sql", Verb: "create"}, "T-02"},
		{Write{Repo: "user-service", Path: "db/migrations/001.sql", Verb: "delete"}, ""},
		{Write{Repo: "user-service", Path: "src/billing/x.ts", Verb: "edit"}, ""},
		{Write{Repo: "user-service", Path: "src/validation/email.ts", Verb: "edit"}, ""},
		{Write{Repo: "order-service", Path: "src/profile/x.ts", Verb: "edit"}, ""},
		{Write{Repo: "user-service", Verb: "push"}, "T-03"},
		{Write{Repo: "user-service", Verb: "comment"}, ""},
	}
	for repo, want := range map[string]bool{"user-service": true, "order-service": false, "": false} {
		if p.WritesIn(repo) != want {
			t.Errorf("WritesIn(%q) = %v", repo, !want)
		}
	}
	// A task with no paths changes no code (ADR 027): its repo needs no worktree or preflight.
	p.Tasks = append(p.Tasks, Task{ID: "T-09", Repo: "notifications", Verbs: []string{"comment"}})
	if p.WritesIn("notifications") {
		t.Error("a repo with only no-code tasks is not written in")
	}
	for _, tt := range tests {
		got := ""
		if task := p.Match(tt.w); task != nil {
			got = task.ID
		}
		if got != tt.want {
			t.Errorf("Match(%+v) = %q, want %q", tt.w, got, tt.want)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"src/**", "src/a/b.go", true}, {"src/**", "src", true}, {"**", "a/b/c", true}, {"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true}, {"src/*.go", "src/a.go", true}, {"src/*.go", "src/a/b.go", false},
		{"src/*_test.go", "src/x_test.go", true}, {"a/**/z", "a/z", true}, {"a/**/z", "a/b/c/z", true},
		{"a/**/z", "a/b/c/y", false}, {"exact.txt", "exact.txt", true}, {"exact.txt", "other.txt", false},
		{"*", "", true}, {"src/**", "lib/a.go", false}, {"x*y*z", "xAyBz", true}, {"x*y*z", "xAz", false},
	}
	for _, c := range cases {
		if MatchGlob(c.pattern, c.path) != c.want {
			t.Errorf("MatchGlob(%q, %q) != %v", c.pattern, c.path, c.want)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := Decode(schema.NewRegistry(), []byte("a: [")); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("bad yaml: %v", err)
	}
}

// TestDeferredTaskGrantsNothing: a deferred task is a later change (doc 04 rule 3), so in this
// change its paths allow no write (rule 7) and do not make a repo one the plan writes in.
func TestDeferredTaskGrantsNothing(t *testing.T) {
	p := &Plan{Tasks: []Task{
		{ID: "T-01", Repo: "users", Paths: []string{"api/**"}, Verbs: []string{"edit", "commit"}},
		{ID: "T-02", Repo: "users", Paths: []string{"legacy/**"}, Verbs: []string{"edit", "delete"}, Kind: KindContract, Deferred: true},
		{ID: "T-03", Repo: "orders", Paths: []string{"clients/**"}, Verbs: []string{"edit"}, Kind: KindContract, Deferred: true},
	}}
	tests := []struct {
		w    Write
		want string
	}{
		{Write{Repo: "users", Path: "api/user.go", Verb: "edit"}, "T-01"},
		{Write{Repo: "users", Path: "legacy/v1.go", Verb: "edit"}, ""},
		{Write{Repo: "users", Path: "legacy/v1.go", Verb: "delete"}, ""},
		{Write{Repo: "orders", Path: "clients/users.go", Verb: "edit"}, ""},
	}
	for _, tt := range tests {
		got := ""
		if m := p.Match(tt.w); m != nil {
			got = m.ID
		}
		if got != tt.want {
			t.Errorf("Match(%+v) = %q, want %q", tt.w, got, tt.want)
		}
	}
	if !p.WritesIn("users") || p.WritesIn("orders") {
		t.Error("only built tasks make a repo one the plan writes in")
	}
	if got := len(p.Built()); got != 1 {
		t.Errorf("Built() has %d tasks, want 1", got)
	}
}

func TestUpstream(t *testing.T) {
	src := []string{"src/**"}
	p := &Plan{Tasks: []Task{
		{ID: "T-01", Repo: "users", Kind: KindExpand, Paths: src},
		{ID: "T-02", Repo: "payments", Kind: KindExpand, Paths: src},
		{ID: "T-03", Repo: "orders", Kind: KindInternal, DependsOn: []string{"T-02"}, Paths: src},
		{ID: "T-04", Repo: "orders", Kind: KindConsume, DependsOn: []string{"T-03", "T-01"}, Paths: src},
		{ID: "T-05", Repo: "gateway", Kind: KindConsume, DependsOn: []string{"T-04"}, Paths: src},
		{ID: "T-06", Repo: "users", Kind: KindContract, Deferred: true, Paths: src},
		{ID: "T-07", Repo: "orders", Kind: KindInternal, DependsOn: []string{"T-06"}, Paths: src},
		{ID: "T-08", Repo: "tickets", Kind: KindInternal, DependsOn: []string{"T-01"}},
		{ID: "T-09", Repo: "orders", Kind: KindInternal, DependsOn: []string{"T-08"}, Paths: src},
	}}
	ids := func(ts []Task) string {
		var out []string
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return strings.Join(out, ",")
	}
	for id, want := range map[string]string{"T-04": "T-01,T-02", "T-05": "T-04", "T-01": "", "T-07": "", "T-99": "", "T-09": "T-01"} {
		if got := ids(p.Upstream(id)); got != want {
			t.Errorf("Upstream(%s) = %q, want %q", id, got, want)
		}
	}
}
