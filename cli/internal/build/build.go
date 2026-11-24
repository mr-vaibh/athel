// Package build is doc 06's build loop bookkeeping: which plan tasks may start (FR-BUILD-001:
// "a task starts only when its dependencies are done") and each task's status, kept in
// change.yaml so the approved plan file never changes (M4-plan G9).
package build

import (
	"slices"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// Task statuses.
const (
	InProgress = "in_progress"
	Done       = "done"
	Blocked    = "blocked"
)

// Task is one plan task's build record.
type Task struct {
	ID      string   `yaml:"id" json:"id"`
	Status  string   `yaml:"status" json:"status"`
	Started string   `yaml:"started_at" json:"started_at"`
	DoneAt  *string  `yaml:"done_at" json:"done_at"`
	Commits []string `yaml:"commits" json:"commits"`
}

func record(recs []Task, id string) (int, bool) {
	i := slices.IndexFunc(recs, func(t Task) bool { return t.ID == id })
	return i, i >= 0
}

func done(recs []Task, id string) bool {
	i, ok := record(recs, id)
	return ok && recs[i].Status == Done
}

// Ready lists the tasks that may start now, in plan order: not started, every dependency done.
// A deferred task is a separate, later change (doc 04 rule 3) and is never ready.
func Ready(p *plan.Plan, recs []Task) []string {
	var out []string
	for _, t := range p.Tasks {
		if _, started := record(recs, t.ID); started || t.Deferred {
			continue
		}
		if !slices.ContainsFunc(t.DependsOn, func(d string) bool { return !done(recs, d) }) {
			out = append(out, t.ID)
		}
	}
	return out
}

// Next is the task to work on now: a task in progress first, then the first ready one, in plan
// order, skipping tasks an open card blocks. Work goes on with unblocked tasks, in other repos
// too, while a card waits (doc 08, decided rules; M5-plan G12). "" means nothing can go on.
func Next(p *plan.Plan, recs []Task, blocked map[string]bool) string {
	for _, t := range p.Tasks {
		if i, ok := record(recs, t.ID); ok && recs[i].Status == InProgress && !blocked[t.ID] {
			return t.ID
		}
	}
	for _, id := range Ready(p, recs) {
		if !blocked[id] {
			return id
		}
	}
	return ""
}

// AllDone reports whether every task built in this change is done; deferred tasks are not.
func AllDone(p *plan.Plan, recs []Task) bool {
	return !slices.ContainsFunc(p.Tasks, func(t plan.Task) bool { return !t.Deferred && !done(recs, t.ID) })
}

// Start records a task as in progress. A task already in progress stays as it is; a done task
// or one whose dependencies are not done is refused.
func Start(p *plan.Plan, recs []Task, id, at string) ([]Task, error) {
	i := slices.IndexFunc(p.Tasks, func(t plan.Task) bool { return t.ID == id })
	if i < 0 {
		return nil, aerr.New(aerr.StateNotFound, "the plan has no task "+id)
	}
	if p.Tasks[i].Deferred {
		return nil, aerr.New(aerr.StateInvalid, id+" is deferred: a separate, later change after consumers deploy (doc 04 rule 3)")
	}
	if j, ok := record(recs, id); ok {
		if recs[j].Status == Done {
			return nil, aerr.New(aerr.StateInvalid, id+" is done")
		}
		return recs, nil
	}
	var waiting []string
	for _, d := range p.Tasks[i].DependsOn {
		if !done(recs, d) {
			waiting = append(waiting, d)
		}
	}
	if len(waiting) > 0 {
		return nil, aerr.New(aerr.StateInvalid, id+" waits for "+strings.Join(waiting, ", ")+" (FR-BUILD-001)").
			WithNextAction("athel build next lists the tasks that may start")
	}
	return append(slices.Clone(recs), Task{ID: id, Status: InProgress, Started: at, Commits: []string{}}), nil
}

// Finish records a task as done with its commits.
func Finish(recs []Task, id string, commits []string, at string) ([]Task, error) {
	i, ok := record(recs, id)
	if !ok || recs[i].Status != InProgress {
		return nil, aerr.New(aerr.StateInvalid, id+" is not in progress")
	}
	out := slices.Clone(recs)
	out[i].Status, out[i].DoneAt, out[i].Commits = Done, &at, append([]string{}, commits...)
	return out, nil
}
