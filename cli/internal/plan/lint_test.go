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
  - {id: T-01, title: Add phone_number column, repo: users, kind: migrate, paths: ["migrations/**"], verbs: [create, edit, test, commit], covers: [AC-1], depends_on: [], size: S, gates: [build, unit], rollback: drop the new column}
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

// TestLintTaskWithoutCode (ADR 027): a task with no paths may cover a criterion that a code task
// also covers, with its test.
func TestLintTaskWithoutCode(t *testing.T) {
	doc := planV2 + "  - {id: T-05, title: Confirm the rollout window, repo: users, kind: internal, paths: [], verbs: [comment], covers: [AC-1], depends_on: [], size: S, gates: [], env_actions: [comment on SHOP-12 with the window]}\n"
	if p := lintPlan(t, doc, "MEDIUM"); len(p) != 0 {
		t.Errorf("problems: %v", p)
	}
}

func TestLintProblems(t *testing.T) {
	tests := map[string]struct{ from, to, want string }{
		"task covers nothing":                   {"covers: [AC-4]", "covers: []", "T-04 covers no acceptance criterion"},
		"criterion uncovered":                   {"covers: [AC-3]", "covers: [AC-2]", "AC-3 is covered by no task"},
		"unknown criterion":                     {"covers: [AC-4]", "covers: [AC-4, AC-9]", "T-04 covers AC-9, which no spec"},
		"consumer before provider":              {"depends_on: [T-02], size: M, gates: [build, unit]}", "depends_on: [], size: M, gates: [build, unit]}", "T-03 moves a consumer but depends on no expand task"},
		"contract not deferred":                 {"kind: contract, deferred: true", "kind: contract", "mark it deferred"},
		"contract before consumer":              {"depends_on: [T-03, T-01]", "depends_on: [T-01]", "T-04 must come after T-03"},
		"large without reason":                  {"size: S, gates: [build, unit], rollback: drop", "size: L, gates: [build, unit], rollback: drop", "T-01 is size L without a size_reason"},
		"behaviour without test":                {"verbs: [edit, test, commit], covers: [AC-3]", "verbs: [edit, commit], covers: [AC-3]", "T-03 changes behaviour without the test verb"},
		"creates without edit":                  {"verbs: [create, edit, test, commit]", "verbs: [create, test, commit]", "T-01 creates files without the edit verb"},
		"deploy in env actions":                 {"env_actions: [comment on SHOP-12]", "env_actions: [deploy users to staging]", `env action "deploy users to staging" deploys`},
		"risky without rollback":                {", rollback: flag users_v2_events off", "", "T-02 (expand) needs a rollback at HIGH risk"},
		"criterion only in a task without code": {`paths: ["api/**"]`, "paths: []", "AC-4 is covered only by tasks that change no code"},
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
	// T-04 is deferred: listed, but a later change's work, so not in this change's estimate.
	if len(s.Repos) != 2 || s.Repos[0].Repo != "users" || !s.Repos[0].Writes || len(s.Repos[0].Tasks) != 3 || s.Repos[1].Repo != "notifications" ||
		strings.Join(s.Gates, ",") != "build,unit,contract" || s.EnvActions[0] != "T-02: comment on SHOP-12" ||
		s.Hours != 3.5 || s.USD != 14 || s.Risk != "HIGH" || s.Assumptions == nil {
		t.Errorf("summary: %+v", s)
	}
	checkDiff(t, p)
}

func TestSummaryRepoWithoutCode(t *testing.T) {
	p, err := Decode(schema.NewRegistry(), []byte(planV2))
	if err != nil {
		t.Fatal(err)
	}
	p.Tasks = append(p.Tasks, Task{ID: "T-05", Title: "Ask the gateway team", Repo: "gateway", Verbs: []string{"comment"}})
	if s := Summarise(p, "HIGH", nil); len(s.Repos) != 3 || s.Repos[2].Repo != "gateway" || s.Repos[2].Writes {
		t.Errorf("a repo with only no-code tasks writes nothing: %+v", s.Repos)
	}
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

// TestLintNewFileNeedsCreate: a task that names a file its repo does not have yet must be allowed
// to create it, or the build stops at the first write (the file is checked on the base branch
// where Athel's cache clone knows it; an unknown repo or a glob is not checked).
func TestLintNewFileNeedsCreate(t *testing.T) {
	doc := planV2 + "  - {id: T-05, title: Gate the routes, repo: users, kind: internal, paths: [main.go, main_test.go, \"cmd/**\"], verbs: [edit, test, commit], covers: [AC-2], depends_on: [], size: S, gates: [build, unit]}\n"
	p, err := Decode(schema.NewRegistry(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var asked []string
	exists := func(repo, path string) (bool, bool) {
		asked = append(asked, repo+":"+path)
		switch {
		case repo == "notifications":
			return false, false
		case path == "main_test.go":
			return false, true
		}
		return true, true
	}
	got := strings.Join(Lint(p, LintContext{Criteria: []string{"AC-1", "AC-2", "AC-3", "AC-4"}, Risk: "HIGH", Exists: exists}), "; ")
	if !strings.Contains(got, "T-05 names main_test.go, which users does not have yet, without the create verb") || strings.Count(got, "does not have yet") != 1 {
		t.Errorf("problems: %s", got)
	}
	if strings.Join(asked, ",") != "users:main.go,users:main_test.go" {
		t.Errorf("only literal paths of tasks without create are checked: %v", asked)
	}
}

// isTest and migrationDown stand in for the gates' matchers (ADR 039).
func isTest(_, p string) bool { return strings.HasSuffix(p, "_test.go") }

func migrationDown(p string) (string, bool) {
	if !strings.HasPrefix(p, "migrations/") || !strings.HasSuffix(p, ".sql") || strings.HasSuffix(p, ".down.sql") {
		return "", false
	}
	if base, ok := strings.CutSuffix(p, ".up.sql"); ok {
		return base + ".down.sql", true
	}
	return "", true
}

// The M5 full eval 3 phone-e164 plans, reduced: a migration task and the API task after it.
const planMigration = `schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: Add phone_e164, repo: users, kind: migrate, paths: [migrations/002_phone.up.sql, migrations/002_phone.down.sql, migrations/migrations_test.go], verbs: [create, edit, test, commit], covers: [AC-1], depends_on: [], size: S, gates: [build, unit], rollback: drop the column}
  - {id: T-02, title: Return phone_e164, repo: users, kind: expand, paths: [api/user.go, api/user_test.go], verbs: [edit, test, commit], covers: [AC-1, AC-2], depends_on: [T-01], size: S, gates: [build, unit], rollback: revert}
  - {id: T-03, title: Wire the route, repo: users, kind: internal, paths: [main.go], verbs: [edit, test, commit], covers: [AC-2], depends_on: [T-02], size: S, gates: [build, unit]}
`

// TestLintTestsAndMigrations (ADR 039): every criterion a code task covers can be named by a test
// that task or an earlier one of its repo writes (the ac-tests gate), and a new migration is an
// up file with its down file in the same task (the migration gate).
func TestLintTestsAndMigrations(t *testing.T) {
	lint := func(doc string, ctx LintContext) string {
		t.Helper()
		p, err := Decode(schema.NewRegistry(), []byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		ctx.Criteria = []string{"AC-1", "AC-2"}
		return strings.Join(Lint(p, ctx), "; ")
	}
	full := LintContext{IsTest: isTest, MigrationDown: migrationDown}
	if got := lint(planMigration, full); got != "" {
		t.Errorf("T-03's AC-2 is tested by T-02, an earlier task of users: %s", got)
	}
	for name, tt := range map[string]struct{ from, to, want string }{
		"migration without its test": {", migrations/migrations_test.go]", "]",
			"T-01 covers AC-1, but neither it nor an earlier users task covering AC-1 has a test file in its paths"},
		"single-file migration": {"migrations/002_phone.up.sql, migrations/002_phone.down.sql", "migrations/002_phone.sql",
			"T-01 names the migration migrations/002_phone.sql, which the migration gate cannot run down: name it NNN_name.up.sql and add NNN_name.down.sql to T-01's paths"},
		"up without down": {"migrations/002_phone.up.sql, migrations/002_phone.down.sql", "migrations/002_phone.up.sql",
			"T-01 names migrations/002_phone.up.sql without its down migration migrations/002_phone.down.sql"},
		"test in a later task": {"depends_on: [T-02], size: S", "depends_on: [], size: S",
			"T-03 covers AC-2, but neither it nor an earlier users task covering AC-2 has a test file"},
	} {
		doc := strings.Replace(planMigration, tt.from, tt.to, 1)
		if doc == planMigration {
			t.Fatalf("%s: the replacement did not apply", name)
		}
		if got := lint(doc, full); !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q lacks %q", name, got, tt.want)
		}
	}
	globs := strings.NewReplacer("migrations/002_phone.up.sql, migrations/002_phone.down.sql, migrations/migrations_test.go", `"migrations/**"`).Replace(planMigration)
	if got := lint(globs, full); got != "" {
		t.Errorf("a glob may hold the test and the down file: %s", got)
	}
	deferred := planMigration + "  - {id: T-04, title: Drop phone, repo: users, kind: contract, deferred: true, paths: [migrations/003_drop.sql], verbs: [delete, commit], covers: [AC-2], depends_on: [T-02], size: S, gates: [build], rollback: revert}\n"
	if got := lint(deferred, full); got != "" {
		t.Errorf("a deferred task is planned again in its own change: %s", got)
	}
	if got := lint(strings.Replace(planMigration, ", migrations/migrations_test.go]", "]", 1), LintContext{}); got != "" {
		t.Errorf("without matchers nothing is checked: %s", got)
	}
}

// TestSummaryDeferredTask: a deferred task is shown as a later change and not estimated.
func TestSummaryDeferredTask(t *testing.T) {
	p, err := Decode(schema.NewRegistry(), []byte(planV2))
	if err != nil {
		t.Fatal(err)
	}
	before := Summarise(p, "HIGH", nil)
	p.Tasks = append(p.Tasks, Task{ID: "T-05", Title: "Remove v1", Repo: "payments", Kind: KindContract, Paths: []string{"v1/**"}, Size: "M", Deferred: true})
	s := Summarise(p, "HIGH", nil)
	last := s.Repos[len(s.Repos)-1]
	if last.Repo != "payments" || last.Writes || last.Tasks[0] != "T-05 Remove v1 (deferred: a later change)" || s.Hours != before.Hours || s.USD != before.USD || s.MergeOrder == nil {
		t.Errorf("summary with a deferred task: %+v", s)
	}
}
