package changeset

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/plan"
)

func TestSteps(t *testing.T) {
	tests := []struct {
		name        string
		tasks       []plan.Task
		coordinated bool
		want        string
	}{
		{"one repo", []plan.Task{task("T-01", "users", plan.KindInternal)}, false, "users"},
		{"a chain", []plan.Task{
			task("T-01", "users", plan.KindExpand), task("T-02", "orders", plan.KindConsume, "T-01"),
		}, false, "users | orders,users"},
		{"two consumers share a step", []plan.Task{
			task("T-01", "users", plan.KindExpand), task("T-02", "orders", plan.KindConsume, "T-01"),
			task("T-03", "gateway", plan.KindConsume, "T-01"), deferred(task("T-04", "users", plan.KindContract, "T-02", "T-03")),
		}, false, "users | gateway,users | orders,users | gateway,orders,users"},
		{"independent repos", []plan.Task{task("T-01", "users", plan.KindInternal), task("T-02", "orders", plan.KindInternal)}, false,
			"orders | users | orders,users"},
		{"a coordinated release: the final state only", []plan.Task{
			task("T-01", "users", plan.KindInternal), task("T-02", "orders", plan.KindInternal, "T-01"),
		}, true, "orders,users"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Derive(&plan.Plan{Tasks: tt.tasks}, tt.coordinated)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, st := range s.Steps() {
				got = append(got, strings.Join(st, ","))
			}
			if strings.Join(got, " | ") != tt.want {
				t.Errorf("got %s, want %s", strings.Join(got, " | "), tt.want)
			}
		})
	}
	if (&Set{}).Steps() != nil {
		t.Error("an empty set has no steps")
	}
}
