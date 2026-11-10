// Package trace builds a change's traceability (doc 05 §Spec format: "PRD ID → REQ → AC → task
// → commit trailer → test name"; FR-SPEC-003, FR-PLAN-002) and finds every missing link. M3
// fills PRD to task; commits and test names are filled from M4 on, when tasks produce them.
package trace

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/prd"
	"github.com/mr-vaibh/athel/cli/internal/spec"
)

// Row is one acceptance criterion with what it traces to.
type Row struct {
	PRD     []string `json:"prd"`
	Service string   `json:"service"`
	Req     string   `json:"req"`
	AC      string   `json:"ac"`
	Tasks   []string `json:"tasks"`
}

// Trace is traceability.md's content.
type Trace struct {
	Change   string   `json:"change"`
	Rows     []Row    `json:"rows"`
	Missing  []string `json:"missing"`
	Uncited  []string `json:"uncited_prd"`
	Complete bool     `json:"complete"`
}

// Build links the PRD index, the specs and the plan (nil before planning). Missing links make
// the trace incomplete; PRD statements no requirement cites are listed for the reader, since a
// PRD may hold context that needs no requirement.
func Build(change string, x prd.Index, specs []*spec.Spec, p *plan.Plan) Trace {
	t := Trace{Change: change, Rows: []Row{}, Missing: []string{}, Uncited: []string{}}
	cited := map[string]bool{}
	tasksOf := map[string][]string{}
	if p != nil {
		for _, task := range p.Tasks {
			for _, ac := range task.Covers {
				tasksOf[ac] = append(tasksOf[ac], task.ID)
			}
		}
	}
	if len(specs) == 0 {
		t.Missing = append(t.Missing, "no spec yet")
	}
	for _, s := range specs {
		t.addSpec(s, x, tasksOf, p != nil, cited)
	}
	if p == nil {
		t.Missing = append(t.Missing, "no plan yet")
	}
	for _, it := range x.Items {
		if it.Kind == "statement" && !cited[it.ID] {
			t.Uncited = append(t.Uncited, it.ID+" "+it.Text)
		}
	}
	slices.Sort(t.Missing)
	t.Missing = slices.Compact(t.Missing)
	t.Complete = len(t.Missing) == 0
	return t
}

// addSpec adds one spec's rows and missing links; planned says whether tasks are expected yet.
func (t *Trace) addSpec(s *spec.Spec, x prd.Index, tasksOf map[string][]string, planned bool, cited map[string]bool) {
	reqs := map[string]spec.Requirement{}
	for _, r := range s.Requirements {
		reqs[r.ID] = r
		for _, id := range r.PRD {
			cited[id] = true
			if !x.Has(id) {
				t.Missing = append(t.Missing, r.ID+" cites "+id+", which the current PRD does not have")
			}
		}
	}
	for _, c := range s.Criteria {
		for _, req := range c.Req {
			row := Row{PRD: reqs[req].PRD, Service: s.Service, Req: req, AC: c.ID, Tasks: tasksOf[c.ID]}
			if row.Tasks == nil {
				row.Tasks = []string{}
				if planned {
					t.Missing = append(t.Missing, c.ID+" is covered by no task")
				}
			}
			t.Rows = append(t.Rows, row)
		}
	}
}

// Render writes traceability.md.
func Render(t Trace) []byte {
	var b bytes.Buffer
	status := "complete"
	if !t.Complete {
		status = "incomplete"
	}
	fmt.Fprintf(&b, "# Traceability · %s · %s\n\n", t.Change, status)
	b.WriteString("| PRD | Service | REQ | AC | Tasks | Commit | Test |\n|---|---|---|---|---|---|---|\n")
	for _, r := range t.Rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | (M4) | (M4) |\n", strings.Join(r.PRD, ", "), r.Service, r.Req, r.AC, strings.Join(r.Tasks, ", "))
	}
	if len(t.Missing) > 0 {
		b.WriteString("\n## Missing links\n")
		for _, m := range t.Missing {
			b.WriteString("- " + m + "\n")
		}
	}
	if len(t.Uncited) > 0 {
		b.WriteString("\n## PRD statements no requirement cites\n")
		for _, u := range t.Uncited {
			b.WriteString("- " + u + "\n")
		}
	}
	return b.Bytes()
}
