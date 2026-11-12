package schema

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// doc 05's spec example (§Spec format), as a spec@1 block.
const specExample = `schema: athel/spec@1
change: CHG-042
service: user-service
repo: github.com/example/user-service
requirements:
  - id: REQ-1
    prd: [PRD-2.1]
    text: When a signed-in customer submits a new phone number, the user-service shall validate it as E.164 and store it.
    decisions: [D-1]
  - id: REQ-2
    prd: [PRD-2.3]
    text: When the phone number changes, the user-service shall publish UserUpdated v2 with the new number within 1 second.
    decisions: []
interfaces_changed:
  - {kind: endpoint, name: "PUT /v1/users/{id}/phone", change: new, description: "request, response, errors"}
  - {kind: event, name: UserUpdated v2, change: additive, description: adds phone_number; v1 unchanged}
data_changes:
  - {table: users, change: add_column, description: "phone_number, nullable"}
  - {table: users, change: backfill, description: phone_number from phone}
design_notes: []
acceptance_criteria:
  - {id: AC-1, req: [REQ-1], given: a customer with phone +919800000000, when: they submit +919811111111, then: GET returns +919811111111}
  - {id: AC-2, req: [REQ-1], given: any customer, when: they submit "98111 11111", then: 400 with code PHONE_INVALID}
out_of_scope: [Removing the old phone field (follow-up change)]
assumptions: []
`

// doc 05's card example (§Card format), plus a choice Athel made alone.
const decisionExample = `schema: athel/decision@1
change: CHG-042
decisions:
  - id: D-1
    kind: card
    rule: A1
    step: spec
    question: Which phone formats should the API accept?
    context: PRD-2.1 says "valid phone number". Current code accepts free text. 14% of stored numbers are not E.164 (scout, F-0210).
    options:
      - {key: a, label: E.164 only, consequence: Clean data; mobile app must format before sending.}
      - {key: b, label: E.164 plus local formats normalised at the API, consequence: Friendlier; parser adds a dependency and edge cases.}
    recommendation: {option: b, reason: App already sends local formats (traces show 61% non-E.164 requests).}
    default_if_unanswered: b
    default_safe: true
    blocks: [T-03, T-04]
    presented_at: null
    answer: null
  - id: D-2
    kind: decision
    step: plan
    question: Where does the phone validator live?
    choice: src/validation/phone.ts, next to the existing validators
    reason: conventions.md puts validators under src/validation
    decided_by: athel
    at: "2026-11-10T10:00:00.000Z"
`

const prdIndexExample = `schema: athel/prd-index@1
change: CHG-042
version: v1
source_sha256: sha256:` + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" + `
items:
  - {id: PRD-2, kind: heading, line: 5, text: Phone number}
  - {id: PRD-2.1, kind: statement, line: 6, text: Customers must be able to change their phone number to a valid phone number.}
`

const prdReviewExample = `schema: athel/prd-review@1
change: CHG-042
version: v1
checklist:
  goal: {status: present, prd: [PRD-1.1]}
  actors: {status: present, prd: [PRD-1.2]}
  scope: {status: present, prd: [PRD-2]}
  out_of_scope: {status: missing, prd: [], note: nothing says what is excluded}
  functional_requirements: {status: present, prd: [PRD-2.1, PRD-2.3]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: partial, prd: [PRD-3.1], note: '"fast" has no number'}
  constraints: {status: present, prd: [PRD-4.1]}
  dependencies: {status: missing, prd: []}
ambiguities:
  - {prd: PRD-3.1, phrase: fast, material: true, resolution: card, ref: D-3}
  - {prd: PRD-2.1, phrase: valid phone number, material: true, resolution: card, ref: D-1}
contradictions:
  - {prd: [PRD-3.1, PRD-4.2], text: "200 ms in one place, 1 s in another", card: D-4}
assumptions:
  - {id: A-1, text: Only signed-in customers change their own number, prd: [PRD-1.2]}
`

func TestM3SchemasValidExamples(t *testing.T) {
	reg := NewRegistry()
	for id, doc := range map[string]string{Spec: specExample, Decision: decisionExample, PRDIndex: prdIndexExample, PRDReview: prdReviewExample} {
		if err := reg.ValidateYAML(id, []byte(doc)); err != nil {
			t.Errorf("%s example: %v", id, err)
		}
	}
}

func TestM3SchemasRejectInvalid(t *testing.T) {
	reg := NewRegistry()
	oneOption := strings.Replace(decisionExample, "      - {key: b, label: E.164 plus local formats normalised at the API, consequence: Friendlier; parser adds a dependency and edge cases.}\n", "", 1)
	five := strings.Replace(decisionExample, "    recommendation:", `      - {key: c, label: c, consequence: c}
      - {key: d, label: d, consequence: d}
      - {key: e, label: e, consequence: e}
    recommendation:`, 1)
	tests := []struct {
		name, id, doc string
	}{
		{"card with one option", Decision, oneOption},
		{"card with five options", Decision, five},
		{"card without a rule", Decision, strings.Replace(decisionExample, "    rule: A1\n", "", 1)},
		{"unknown ask rule", Decision, strings.Replace(decisionExample, "rule: A1", "rule: A8", 1)},
		{"decision not by athel", Decision, strings.Replace(decisionExample, "decided_by: athel", "decided_by: architect", 1)},
		{"answer by the model", Decision, strings.Replace(decisionExample, "    answer: null\n", "    answer: {option: b, by: architect, at: \"2026-11-10T10:00:00.000Z\"}\n", 1)},
		{"requirement without a PRD ID", Spec, strings.Replace(specExample, "prd: [PRD-2.1]", "prd: []", 1)},
		{"criterion without then", Spec, strings.Replace(specExample, ", then: GET returns +919811111111", "", 1)},
		{"criterion citing no requirement", Spec, strings.Replace(specExample, "req: [REQ-1], given: a customer", "req: [], given: a customer", 1)},
		{"unknown contract change", Spec, strings.Replace(specExample, "change: additive", "change: minor", 1)},
		{"malformed PRD ID", PRDIndex, strings.Replace(prdIndexExample, "PRD-2.1", "PRD-2.1.4", 1)},
		{"checklist item missing", PRDReview, strings.Replace(prdReviewExample, "  dependencies: {status: missing, prd: []}\n", "", 1)},
		{"contradiction with one PRD ID", PRDReview, strings.Replace(prdReviewExample, "prd: [PRD-3.1, PRD-4.2]", "prd: [PRD-3.1]", 1)},
	}
	for _, tt := range tests {
		if err := reg.ValidateYAML(tt.id, []byte(tt.doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v, want E-STATE-SCHEMA", tt.name, err)
		}
	}
}
