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
