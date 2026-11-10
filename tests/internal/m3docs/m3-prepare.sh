# shellcheck shell=bash
# m3_prepare does the M3 spec step's work for a change whose PRD is "# PRD" (CHG-001), so a shell
# demo or live check reaches specified and planned through the step guards (ADR 021), not around
# them. It mirrors m3docs.go; TestShellMirrorsGo keeps the two equal.
# Usage: m3_prepare <command that runs athel with its arguments> <scratch folder>
m3_prepare() {
  local run="$1" dir="$2"
  cat >"$dir/m3-review.yaml" <<'M3_REVIEW'
schema: athel/prd-review@1
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
M3_REVIEW
  cat >"$dir/m3-conventions.yaml" <<'M3_CONVENTIONS'
conventions: |
  # Conventions
  Go, table-driven tests.
map: |
  # Map
  src/ holds the code.
M3_CONVENTIONS
  cat >"$dir/m3-spec.yaml" <<'M3_SPEC'
schema: athel/spec@1
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
M3_SPEC
  cat >"$dir/m3-plan.yaml" <<'M3_PLAN'
schema: athel/plan@2
change: CHG-001
hash: null
tasks:
  - {id: T-01, title: phone validation, repo: svc, kind: internal, paths: ["src/**"], verbs: [edit, test], covers: [AC-1], depends_on: [], size: S, gates: []}
M3_PLAN
  "$run" prd review save CHG-001 "$dir/m3-review.yaml" >/dev/null
  "$run" conventions save github.com/example/svc "$dir/m3-conventions.yaml" --new >/dev/null
  "$run" spec save CHG-001 "$dir/m3-spec.yaml" >/dev/null
  "$run" plan set CHG-001 "$dir/m3-plan.yaml" >/dev/null
}
