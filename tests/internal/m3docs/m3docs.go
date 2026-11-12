// Package m3docs holds the documents an end-to-end test needs to take a change through M3's
// step guards (ADR 021): a PRD review, conventions, a spec and a plan@2 for a change whose PRD
// is the one-heading "# PRD" (indexed as PRD-1). Scenario and red-team suites use it so they
// reach planned the way a real change does, not around the guards.
package m3docs

import "strings"

// PRD is the change's PRD text.
const PRD = "# PRD"

// Review is the PRD review `athel prd review save CHG-001 -` reads on stdin.
const Review = `schema: athel/prd-review@1
change: CHG-001
version: v1
checklist:
  goal: {status: present, prd: [PRD-1]}
  actors: {status: missing, prd: []}
  scope: {status: missing, prd: []}
  out_of_scope: {status: missing, prd: []}
  functional_requirements: {status: present, prd: [PRD-1]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: missing, prd: []}
  constraints: {status: missing, prd: []}
  dependencies: {status: missing, prd: []}
ambiguities: []
contradictions: []
assumptions: []
`

// Repo is the spec's repo; Conventions are saved for it with --new.
const Repo = "github.com/example/svc"

// Conventions is what `athel conventions save <Repo> - --new` reads on stdin.
const Conventions = "conventions: |\n  # Conventions\n  Go, table-driven tests.\nmap: |\n  # Map\n  src/ holds the code.\n"

// Spec is the one-service spec `athel spec save CHG-001 -` reads on stdin.
const Spec = `schema: athel/spec@1
change: CHG-001
service: svc
repo: github.com/example/svc
requirements:
  - id: REQ-1
    prd: [PRD-1]
    text: When a customer submits a phone number, the svc service shall store it.
    decisions: []
interfaces_changed: []
data_changes: []
design_notes: []
acceptance_criteria:
  - {id: AC-1, req: [REQ-1], given: a customer, when: they submit +919811111111, then: GET returns +919811111111}
out_of_scope: []
assumptions: []
`

// Plan is a plan@2 with one task in repo, covering AC-1, allowed the given verbs and paths.
func Plan(repo string, paths, verbs []string) string {
	return "schema: athel/plan@2\nchange: CHG-001\nhash: null\ntasks:\n" +
		"  - {id: T-01, title: phone validation, repo: " + repo + ", kind: internal, paths: [\"" + strings.Join(paths, `", "`) + "\"], verbs: [" +
		strings.Join(verbs, ", ") + "], covers: [AC-1], depends_on: [], size: S, gates: []}\n"
}
