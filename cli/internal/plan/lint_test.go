package plan

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// An expand-and-contract plan across two services, as doc 04 orders it.
const planV2 = `schema: athel/plan@2
change: CHG-042
hash: null
tasks:
  - {id: T-01, title: Add phone_number column, repo: users, kind: migrate, paths: ["migrations/**"], verbs: [create, test, commit], covers: [AC-1], depends_on: [], size: S, gates: [build, unit], rollback: drop the new column}
  - {id: T-02, title: Publish UserUpdated v2, repo: users, kind: expand, paths: ["events/**", "test/**"], verbs: [edit, test, commit, push], covers: [AC-2], depends_on: [T-01], size: M, gates: [build, unit, contract], rollback: flag users_v2_events off, env_actions: [comment on SHOP-12]}
  - {id: T-03, title: Read UserUpdated v2, repo: notifications, kind: consume, paths: ["src/**"], verbs: [edit, test, commit], covers: [AC-3], depends_on: [T-02], size: M, gates: [build, unit]}
  - {id: T-04, title: Remove the old phone field, repo: users, kind: contract, deferred: true, paths: ["api/**"], verbs: [delete, commit], covers: [AC-4], depends_on: [T-03, T-01], size: S, gates: [build], rollback: revert}
`

func lintPlan(t *testing.T, doc string, risk string) []string {
	t.Helper()
	p, err := Decode(schema.NewRegistry(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return Lint(p, LintContext{Criteria: []string{"AC-1", "AC-2", "AC-3", "AC-4"}, Risk: risk})
}

func TestLintPassesAGoodPlan(t *testing.T) {
	if p := lintPlan(t, planV2, "HIGH"); len(p) != 0 {
		t.Errorf("problems: %v", p)
	}
}

func TestLintProblems(t *testing.T) {
	tests := map[string]struct{ from, to, want string }{
		"task covers nothing":      {"covers: [AC-4]", "covers: []", "T-04 covers no acceptance criterion"},
		"criterion uncovered":      {"covers: [AC-3]", "covers: [AC-2]", "AC-3 is covered by no task"},
		"unknown criterion":        {"covers: [AC-4]", "covers: [AC-4, AC-9]", "T-04 covers AC-9, which no spec"},
		"consumer before provider": {"depends_on: [T-02], size: M, gates: [build, unit]}", "depends_on: [], size: M, gates: [build, unit]}", "T-03 moves a consumer but depends on no expand task"},
		"contract not deferred":    {"kind: contract, deferred: true", "kind: contract", "mark it deferred"},
		"contract before consumer": {"depends_on: [T-03, T-01]", "depends_on: [T-01]", "T-04 must come after T-03"},
		"large without reason":     {"size: S, gates: [build, unit], rollback: drop", "size: L, gates: [build, unit], rollback: drop", "T-01 is size L without a size_reason"},
		"behaviour without test":   {"verbs: [edit, test, commit], covers: [AC-3]", "verbs: [edit, commit], covers: [AC-3]", "T-03 changes behaviour without the test verb"},
		"deploy in env actions":    {"env_actions: [comment on SHOP-12]", "env_actions: [deploy users to staging]", `env action "deploy users to staging" deploys`},
		"risky without rollback":   {", rollback: flag users_v2_events off", "", "T-02 (expand) needs a rollback at HIGH risk"},
	}
	for name, tt := range tests {
		doc := strings.Replace(planV2, tt.from, tt.to, 1)
		if doc == planV2 {
			t.Fatalf("%s: the replacement did not apply", name)
		}
		if got := strings.Join(lintPlan(t, doc, "HIGH"), "; "); !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q lacks %q", name, got, tt.want)
		}
	}
	if got := lintPlan(t, strings.Replace(planV2, ", rollback: flag users_v2_events off", "", 1), "MEDIUM"); len(got) != 0 {
		t.Errorf("rollback is only required at HIGH and CRITICAL: %v", got)
	}
}

func TestSummaryOrderAndDiff(t *testing.T) {
	p, err := Decode(schema.NewRegistry(), []byte(planV2))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, task := range p.Order() {
		ids = append(ids, task.ID)
	}
	if strings.Join(ids, " ") != "T-01 T-02 T-03 T-04" {
		t.Errorf("order: %v", ids)
	}
	s := Summarise(p, "HIGH", nil)
	if len(s.Repos) != 2 || s.Repos[0].Repo != "users" || len(s.Repos[0].Tasks) != 3 || s.Repos[1].Repo != "notifications" ||
		strings.Join(s.Gates, ",") != "build,unit,contract" || s.EnvActions[0] != "T-02: comment on SHOP-12" ||
		s.Hours != 4 || s.USD != 16 || s.Risk != "HIGH" || s.Assumptions == nil {
		t.Errorf("summary: %+v", s)
	}
	checkDiff(t, p)
}

func checkDiff(t *testing.T, p *Plan) {
	t.Helper()
	cur, _ := Decode(schema.NewRegistry(), []byte(strings.Replace(strings.Replace(planV2, "title: Read UserUpdated v2", "title: Read UserUpdated v2 only", 1),
		"  - {id: T-04", "  - {id: T-05, title: Docs, repo: users, kind: internal, paths: [docs/**], verbs: [edit, test], covers: [AC-4], depends_on: [], size: S, gates: [build]}\n  - {id: T-99", 1)))
	d := Compare(p, cur)
	if strings.Join(d.Added, ",") != "T-05,T-99" || strings.Join(d.Removed, ",") != "T-04" || strings.Join(d.Changed, ",") != "T-03" {
		t.Errorf("diff: %+v", d)
	}
}

func TestPlanV1StaysReadable(t *testing.T) {
	v1 := `schema: athel/plan@1
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: x, repo: users, paths: ["**"], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: [build]}
`
	p, err := Decode(schema.NewRegistry(), []byte(v1))
	if err != nil || p.Tasks[0].Kind != "" {
		t.Fatalf("v1: %+v %v", p, err)
	}
	if _, err := Decode(schema.NewRegistry(), []byte(strings.Replace(v1, "plan@1", "plan@2", 1))); err == nil {
		t.Error("a plan@2 task needs a kind")
	}
}
