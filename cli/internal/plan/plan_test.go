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
    verbs: [create, test, commit]
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
		{"unknown verb", "verbs: [create, test, commit]", "verbs: [deploy]", aerr.StateSchema},
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
		{Write{Repo: "user-service", Path: "db/migrations/001.sql", Verb: "edit"}, ""},
		{Write{Repo: "user-service", Path: "src/billing/x.ts", Verb: "edit"}, ""},
		{Write{Repo: "user-service", Path: "src/validation/email.ts", Verb: "edit"}, ""},
		{Write{Repo: "order-service", Path: "src/profile/x.ts", Verb: "edit"}, ""},
		{Write{Repo: "user-service", Verb: "push"}, "T-03"},
		{Write{Repo: "user-service", Verb: "comment"}, ""},
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
