package build

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

func testPlan() *plan.Plan {
	return &plan.Plan{Tasks: []plan.Task{
		{ID: "T-01", Repo: "users"},
		{ID: "T-02", Repo: "users", DependsOn: []string{"T-01"}},
		{ID: "T-03", Repo: "orders"},
		{ID: "T-04", Repo: "users", DependsOn: []string{"T-02", "T-03"}},
	}}
}

func TestReady(t *testing.T) {
	p := testPlan()
	if got := strings.Join(Ready(p, nil), ","); got != "T-01,T-03" {
		t.Errorf("tasks with no dependencies, in plan order: %s", got)
	}
	recs := []Task{{ID: "T-01", Status: Done}, {ID: "T-03", Status: InProgress}}
	if got := strings.Join(Ready(p, recs), ","); got != "T-02" {
		t.Errorf("T-02 once T-01 is done; T-03 is already started: %s", got)
	}
	recs = []Task{{ID: "T-01", Status: Done}, {ID: "T-02", Status: Done}, {ID: "T-03", Status: Done}}
	if got := strings.Join(Ready(p, recs), ","); got != "T-04" {
		t.Errorf("T-04 needs both: %s", got)
	}
	if !AllDone(p, append(recs, Task{ID: "T-04", Status: Done})) || AllDone(p, recs) {
		t.Error("all done only when every task is")
	}
}

func TestStartAndFinish(t *testing.T) {
	p := testPlan()
	recs, err := Start(p, nil, "T-01", "2026-11-14T10:00:00.000Z")
	if err != nil || len(recs) != 1 || recs[0].Status != InProgress || recs[0].Started == "" {
		t.Fatalf("%+v %v", recs, err)
	}
	if again, err := Start(p, recs, "T-01", "later"); err != nil || again[0].Started != recs[0].Started {
		t.Errorf("starting a started task again keeps it: %+v %v", again, err)
	}
	if _, err := Start(p, recs, "T-02", "x"); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "T-01") {
		t.Errorf("a task before its dependencies: %v", err)
	}
	if _, err := Start(p, recs, "T-09", "x"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("an unknown task: %v", err)
	}
	recs, err = Finish(recs, "T-01", []string{"abc"}, "2026-11-14T11:00:00.000Z")
	if err != nil || recs[0].Status != Done || recs[0].Commits[0] != "abc" {
		t.Fatalf("%+v %v", recs, err)
	}
	if _, err := Start(p, recs, "T-01", "x"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("a done task does not start again: %v", err)
	}
	if _, err := Finish(recs, "T-03", nil, "x"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("finishing a task never started: %v", err)
	}
}

// TestDeferredTasksAreNeverBuilt: a deferred removal is a separate, later change (doc 04 rule 3,
// M5-plan G3), so it is never ready, cannot be started, and does not hold the build open.
func TestDeferredTasksAreNeverBuilt(t *testing.T) {
	p := testPlan()
	p.Tasks = append(p.Tasks, plan.Task{ID: "T-05", Title: "remove v1", Repo: p.Tasks[0].Repo, Kind: plan.KindContract, DependsOn: []string{"T-04"}, Deferred: true})
	recs := []Task{{ID: "T-01", Status: Done}, {ID: "T-02", Status: Done}, {ID: "T-03", Status: Done}, {ID: "T-04", Status: Done}}
	if got := Ready(p, recs); len(got) != 0 {
		t.Errorf("a deferred task is never ready: %v", got)
	}
	if !AllDone(p, recs) {
		t.Error("the build is done without its deferred tasks")
	}
	if _, err := Start(p, recs, "T-05", "2026-11-24T10:00:00.000Z"); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "deferred") {
		t.Errorf("starting a deferred task: %v", err)
	}
}

func TestNextSkipsBlockedTasks(t *testing.T) {
	p := testPlan() // T-01 and T-03 have no dependencies; T-02 needs T-01; T-04 needs T-02 and T-03
	tests := []struct {
		name    string
		recs    []Task
		blocked map[string]bool
		want    string
	}{
		{"nothing started", nil, nil, "T-01"},
		{"a task in progress first", []Task{{ID: "T-03", Status: InProgress}}, nil, "T-03"},
		{"a blocked task in progress is passed over", []Task{{ID: "T-01", Status: InProgress}}, map[string]bool{"T-01": true}, "T-03"},
		{"a blocked ready task is passed over", nil, map[string]bool{"T-01": true}, "T-03"},
		{"nothing can go on", []Task{{ID: "T-01", Status: InProgress}, {ID: "T-03", Status: Done}}, map[string]bool{"T-01": true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Next(p, tt.recs, tt.blocked); got != tt.want {
				t.Errorf("Next = %q, want %q", got, tt.want)
			}
		})
	}
}
