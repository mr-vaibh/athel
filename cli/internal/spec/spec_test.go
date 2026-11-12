package spec

import (
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// doc 05's example spec (§Spec format), completed with criteria for every requirement.
const example = `schema: athel/spec@1
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
data_changes:
  - {table: users, change: add_column, description: "phone_number, nullable"}
design_notes: []
acceptance_criteria:
  - {id: AC-1, req: [REQ-1], given: a customer with phone +919800000000, when: they submit +919811111111, then: GET returns +919811111111}
  - {id: AC-2, req: [REQ-1], given: any customer, when: they submit "98111 11111", then: 400 with code PHONE_INVALID}
  - {id: AC-3, req: [REQ-2], given: a customer, when: their number changes, then: UserUpdated v2 is published within 1 second}
out_of_scope: [Removing the old phone field]
assumptions: []
`

func decode(t *testing.T, doc string) *Spec {
	t.Helper()
	s, err := Decode(schema.NewRegistry(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ctx(others ...*Spec) Context {
	return Context{Change: "CHG-042", PRD: func(id string) bool { return id == "PRD-2.1" || id == "PRD-2.3" },
		Decision: func(id string) bool { return id == "D-1" }, Others: others}
}

func TestExamplePasses(t *testing.T) {
	s := decode(t, example)
	if p := Check(s, ctx()); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
	md := string(Render(s, []byte(example)))
	for _, want := range []string{"# Spec: user-service · CHG-042", "- REQ-1 (PRD-2.1): When a signed-in customer", "[Decision D-1]",
		"## Interfaces changed\n- endpoint PUT /v1/users/{id}/phone (new)", "- AC-2 (REQ-1): Given any customer, when they submit \"98111 11111\", then 400 with code PHONE_INVALID.",
		"## Out of scope\n- Removing the old phone field", "```yaml\nschema: athel/spec@1"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q:\n%s", want, md)
		}
	}
}

func TestLintAndLinks(t *testing.T) {
	tests := map[string]struct{ from, to, want string }{
		"not EARS": {"When a signed-in customer submits a new phone number, the user-service shall validate it as E.164 and store it.",
			"Phone numbers get validated.", "REQ-1 is not in EARS form"},
		"another subject":               {"the user-service shall publish", "the notifications service shall publish", `names "notifications service" as its subject`},
		"hedging":                       {"shall validate it as E.164", "shall validate it as E.164 etc.", `uses "etc"`},
		"vague then":                    {"then: GET returns +919811111111", "then: it works", "is not observable"},
		"nothing to see":                {"then: GET returns +919811111111", "then: the customer is happy", "names nothing a test can observe"},
		"number without unit":           {"then: UserUpdated v2 is published within 1 second", "then: UserUpdated v2 is published within 1", `"within 1" has no unit`},
		"unknown PRD ID":                {"prd: [PRD-2.3]", "prd: [PRD-9.9]", "REQ-2 cites PRD-9.9"},
		"unknown decision":              {"decisions: [D-1]", "decisions: [D-7]", "cites decision D-7"},
		"another change":                {"change: CHG-042", "change: CHG-043", "the spec is for CHG-043, not CHG-042"},
		"criterion for nothing":         {"req: [REQ-2]", "req: [REQ-9]", "AC-3 covers REQ-9"},
		"requirement without criterion": {"req: [REQ-2]", "req: [REQ-1]", "REQ-2 has no acceptance criterion"},
		"duplicate ID":                  {"id: AC-3", "id: AC-2", "AC-2 is used twice in this spec"},
	}
	for name, tt := range tests {
		got := strings.Join(Check(decode(t, strings.Replace(example, tt.from, tt.to, 1)), ctx()), "; ")
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s: %q lacks %q", name, got, tt.want)
		}
	}
}

func TestIDsUniqueAcrossServices(t *testing.T) {
	other := decode(t, strings.Replace(example, "service: user-service", "service: notifications", 1))
	if got := strings.Join(Check(decode(t, example), ctx(other)), "; "); !strings.Contains(got, "REQ-1 is already used by the notifications spec") {
		t.Errorf("collision: %s", got)
	}
	same := decode(t, example)
	if got := Check(decode(t, example), ctx(same)); len(got) != 0 {
		t.Errorf("re-saving the same service is not a collision: %v", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	reg := schema.NewRegistry()
	for name, doc := range map[string]string{"unknown field": example + "extra: 1\n", "schema": strings.Replace(example, "change: new", "change: minor", 1)} {
		if _, err := Decode(reg, []byte(doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRenderEmptySections(t *testing.T) {
	s := decode(t, example)
	s.Interfaces, s.DataChanges, s.OutOfScope = nil, nil, nil
	s.DesignNotes = []string{"keep the validator pure"}
	s.Assumptions = []Assumption{{ID: "A-1", Text: "only signed-in customers"}}
	md := string(Render(s, nil))
	for _, want := range []string{"## Interfaces changed\n- none", "## Design notes\n- keep the validator pure", "## Assumptions\n- A-1: only signed-in customers"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q", want)
		}
	}
}

func TestExtract(t *testing.T) {
	reg := schema.NewRegistry()
	got, err := Extract(reg, Render(decode(t, example), []byte(example)))
	if err != nil || got.Service != "user-service" || len(got.Criteria) != 3 {
		t.Fatalf("extract: %+v %v", got, err)
	}
	for name, md := range map[string]string{"no block": "# Spec\n", "unterminated": "```yaml\nschema: x\n"} {
		if _, err := Extract(reg, []byte(md)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
