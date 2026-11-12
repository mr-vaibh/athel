package trace

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/prd"
	"github.com/mr-vaibh/athel/cli/internal/spec"
)

func fixture() (prd.Index, []*spec.Spec, *plan.Plan) {
	x := prd.Build("CHG-001", "v1", "sha256:x", []byte("# Phone\n- Customers must change their number\n- Context only, no requirement must cite this\n"), nil)
	s := &spec.Spec{Service: "users", Requirements: []spec.Requirement{{ID: "REQ-1", PRD: []string{"PRD-1.1"}}},
		Criteria: []spec.Criterion{{ID: "AC-1", Req: []string{"REQ-1"}}, {ID: "AC-2", Req: []string{"REQ-1"}}}}
	p := &plan.Plan{Change: "CHG-001", Tasks: []plan.Task{{ID: "T-01", Covers: []string{"AC-1", "AC-2"}}}}
	return x, []*spec.Spec{s}, p
}

func TestCompleteTrace(t *testing.T) {
	x, specs, p := fixture()
	tr := Build("CHG-001", x, specs, p)
	if !tr.Complete || len(tr.Rows) != 2 || tr.Rows[0].Tasks[0] != "T-01" || len(tr.Uncited) != 1 || !strings.Contains(tr.Uncited[0], "PRD-1.2") {
		t.Fatalf("trace: %+v", tr)
	}
	md := string(Render(tr))
	for _, want := range []string{"# Traceability · CHG-001 · complete", "| PRD-1.1 | users | REQ-1 | AC-1 | T-01 | (M4) | (M4) |", "## PRD statements no requirement cites\n- PRD-1.2"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q:\n%s", want, md)
		}
	}
}

func TestMissingLinks(t *testing.T) {
	x, specs, p := fixture()
	p.Tasks[0].Covers = []string{"AC-1"}
	specs[0].Requirements[0].PRD = []string{"PRD-1.1", "PRD-9.9"}
	tr := Build("CHG-001", x, specs, p)
	if tr.Complete || strings.Join(tr.Missing, "; ") != "AC-2 is covered by no task; REQ-1 cites PRD-9.9, which the current PRD does not have" {
		t.Errorf("missing: %v", tr.Missing)
	}
	if md := string(Render(tr)); !strings.Contains(md, "· incomplete") || !strings.Contains(md, "## Missing links\n- AC-2") {
		t.Errorf("render: %s", md)
	}
	if tr := Build("CHG-001", x, nil, nil); strings.Join(tr.Missing, "; ") != "no plan yet; no spec yet" {
		t.Errorf("nothing yet: %v", tr.Missing)
	}
	if tr := Build("CHG-001", x, specs, nil); strings.Contains(strings.Join(tr.Missing, ";"), "covered by no task") {
		t.Errorf("before planning, tasks are not missing links: %v", tr.Missing)
	}
}
