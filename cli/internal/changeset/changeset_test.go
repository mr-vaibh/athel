package changeset

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

func task(id, repo, kind string, deps ...string) plan.Task {
	return plan.Task{ID: id, Title: "task " + id, Repo: repo, Kind: kind, DependsOn: deps}
}

func deferred(t plan.Task) plan.Task { t.Deferred = true; return t }

// summary renders a set as "repo:role:order<deps" lines, in set order.
func summary(s *Set) string {
	var lines []string
	for _, e := range s.Repos {
		line := e.Repo + ":" + e.Role + ":" + string(rune('0'+e.Order))
		if len(e.DependsOn) > 0 {
			line += "<" + strings.Join(e.DependsOn, ",")
		}
		if e.Deferred {
			line += " deferred"
		}
		lines = append(lines, line)
	}
	return s.Strategy + " " + strings.Join(lines, " ")
}

func TestDeriveRolesAndOrder(t *testing.T) {
	tests := []struct {
		name  string
		tasks []plan.Task
		want  string
	}{
		{"one repo", []plan.Task{task("T-01", "users", plan.KindInternal)}, "single users:internal:1"},
		{"doc 04's example", []plan.Task{
			task("T-01", "users", plan.KindExpand),
			task("T-02", "orders", plan.KindConsume, "T-01"),
			task("T-03", "gateway", plan.KindConsume, "T-01"),
			deferred(task("T-04", "users", plan.KindContract, "T-02", "T-03")),
		}, "expand_contract users:provider:1 gateway:consumer:2<users orders:consumer:2<users users:provider_cleanup:3 deferred"},
		{"migration then expand then consume", []plan.Task{
			task("T-01", "users", plan.KindMigrate),
			task("T-02", "users", plan.KindExpand, "T-01"),
			task("T-03", "orders", plan.KindConsume, "T-02"),
		}, "expand_contract users:provider:1 orders:consumer:2<users"},
		{"a chain through a repo's own tasks", []plan.Task{
			task("T-01", "users", plan.KindExpand),
			task("T-02", "orders", plan.KindInternal),
			task("T-03", "orders", plan.KindConsume, "T-02", "T-01"),
			task("T-04", "gateway", plan.KindConsume, "T-03"),
		}, "expand_contract users:provider:1 orders:consumer:2<users gateway:consumer:3<orders"},
		{"independent repos share step 1", []plan.Task{
			task("T-01", "users", plan.KindInternal),
			task("T-02", "orders", plan.KindInternal),
		}, "expand_contract orders:internal:1 users:internal:1"},
		{"a dependency on a deferred task waits for nothing", []plan.Task{
			deferred(task("T-01", "users", plan.KindContract)),
			task("T-02", "orders", plan.KindInternal, "T-01"),
		}, "expand_contract orders:internal:1 users:provider_cleanup:2 deferred"},
		{"one repo with a deferred cleanup", []plan.Task{
			task("T-01", "users", plan.KindExpand),
			deferred(task("T-02", "users", plan.KindContract, "T-01")),
		}, "expand_contract users:provider:1 users:provider_cleanup:2 deferred"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Derive(&plan.Plan{Tasks: tt.tasks}, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := summary(s); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
			if s.Invariant != Invariant || s.Verify != nil {
				t.Errorf("invariant %q verify %v", s.Invariant, s.Verify)
			}
		})
	}
}

func TestTwoConsumersShareAnOrder(t *testing.T) {
	s, err := Derive(&plan.Plan{Tasks: []plan.Task{
		task("T-01", "users", plan.KindExpand),
		task("T-02", "orders", plan.KindConsume, "T-01"),
		task("T-03", "gateway", plan.KindConsume, "T-01"),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	built := s.Built()
	if len(built) != 3 || built[1].Order != 2 || built[2].Order != 2 {
		t.Errorf("both consumers merge at step 2: %+v", built)
	}
}

func TestDeferredCleanupNotBuilt(t *testing.T) {
	s, err := Derive(&plan.Plan{Tasks: []plan.Task{
		task("T-01", "users", plan.KindExpand),
		task("T-02", "orders", plan.KindConsume, "T-01"),
		deferred(task("T-03", "users", plan.KindContract, "T-02")),
		deferred(task("T-04", "users", plan.KindContract, "T-03")),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	last := s.Repos[len(s.Repos)-1]
	if !last.Deferred || last.Note == nil || *last.Note != "T-03: task T-03; T-04: task T-04 (after consumers deploy)" {
		t.Errorf("one cleanup entry per repo, naming its tasks: %+v", last)
	}
	for _, e := range s.Built() {
		if e.Deferred {
			t.Errorf("Built holds a deferred entry: %+v", e)
		}
	}
}

func TestCoordinatedStrategy(t *testing.T) {
	s, err := Derive(&plan.Plan{Tasks: []plan.Task{task("T-01", "users", plan.KindInternal), task("T-02", "orders", plan.KindInternal, "T-01")}}, true)
	if err != nil || s.Strategy != Coordinated {
		t.Errorf("strategy %v, %v", s, err)
	}
}

func TestTwoWayReposRefused(t *testing.T) {
	_, err := Derive(&plan.Plan{Tasks: []plan.Task{
		task("T-01", "users", plan.KindExpand, "T-04"),
		task("T-02", "orders", plan.KindConsume, "T-01"),
		task("T-03", "orders", plan.KindExpand),
		task("T-04", "users", plan.KindConsume, "T-03"),
	}}, false)
	if !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "users and orders wait on each other (users → orders → users)") {
		t.Errorf("err = %v", err)
	}
}

func TestLines(t *testing.T) {
	s, err := Derive(&plan.Plan{Tasks: []plan.Task{
		task("T-01", "users", plan.KindExpand),
		task("T-02", "orders", plan.KindConsume, "T-01"),
		task("T-03", "gateway", plan.KindConsume, "T-01"),
		task("T-04", "payments", plan.KindInternal, "T-02"),
		deferred(task("T-05", "users", plan.KindContract, "T-02", "T-03")),
	}}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1. users (provider)",
		"2. gateway, orders (consumer; after users; either order)",
		"3. payments (internal; after orders)",
		"later, a separate change: users — T-05: task T-05 (after consumers deploy)",
	}
	if got := s.Lines(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
