package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const specBlock = `schema: athel/spec@1
change: CHG-001
service: users
repo: github.com/example/users
requirements:
  - id: REQ-1
    prd: [PRD-1.1]
    text: When a customer submits a new phone number, the users service shall store it.
    decisions: [D-1]
interfaces_changed: []
data_changes: []
design_notes: []
acceptance_criteria:
  - {id: AC-1, req: [REQ-1], given: a customer, when: they submit +919811111111, then: GET returns +919811111111}
out_of_scope: []
assumptions: []
`

func specSetup(t *testing.T) *harness {
	t.Helper()
	h := prdSetup(t)
	h.stdin = cardYAML
	h.mustOK("decide", "add", "CHG-001", "-")
	return h
}

func TestSpecSaveListShow(t *testing.T) {
	h := specSetup(t)
	h.stdin = specBlock
	if v := h.mustOK("spec", "validate", "CHG-001", "-"); v["valid"] != true {
		t.Fatalf("validate: %v", v)
	}
	saved := h.mustOK("spec", "save", "CHG-001", "-")
	b, err := os.ReadFile(saved["path"].(string))
	if err != nil || !strings.Contains(string(b), "# Spec: users · CHG-001") || !strings.HasSuffix(saved["path"].(string), filepath.Join("spec", "users.md")) {
		t.Fatalf("saved: %v %s %v", saved, b, err)
	}
	h.stdin = strings.NewReplacer("service: users", "service: notifications", "REQ-1", "REQ-2", "AC-1", "AC-2").Replace(specBlock)
	h.stdin = strings.Replace(h.stdin, "the users service shall", "the notifications service shall", 1)
	h.mustOK("spec", "save", "CHG-001", "-")
	if l := asJSON(t, h.mustOK("spec", "list", "CHG-001")); !strings.Contains(l, `"service":"notifications"`) || strings.Index(l, `"notifications"`) > strings.Index(l, `"service":"users"`) {
		t.Errorf("list (sorted by service): %s", l)
	}
	if sp := h.mustOK("spec", "show", "CHG-001", "users"); sp["repo"] != "github.com/example/users" {
		t.Errorf("show: %v", sp)
	}
	h.stdin = strings.Replace(specBlock, "service: users", "service: orders", 1)
	if v := h.mustOK("spec", "validate", "CHG-001", "-"); v["valid"] != false || !strings.Contains(asJSON(t, v["problems"]), "REQ-1 is already used by the users spec") {
		t.Errorf("collision across services: %v", v)
	}
}

func TestSpecErrors(t *testing.T) {
	h := specSetup(t)
	tests := []struct {
		name, stdin string
		args        []string
		code        aerr.Code
	}{
		{"rejected", strings.Replace(specBlock, "prd: [PRD-1.1]", "prd: [PRD-7.7]", 1), []string{"save", "CHG-001", "-"}, aerr.StateInvalid},
		{"schema", "schema: nope\n", []string{"save", "CHG-001", "-"}, aerr.StateSchema},
		{"validate schema", "schema: nope\n", []string{"validate", "CHG-001", "-"}, aerr.StateSchema},
		{"unknown change", specBlock, []string{"validate", "CHG-009", "-"}, aerr.StateNotFound},
		{"show unknown service", "", []string{"show", "CHG-001", "billing"}, aerr.StateNotFound},
		{"list unknown change", "", []string{"list", "CHG-009"}, aerr.StateNotFound},
		{"missing file", "", []string{"save", "CHG-001", filepath.Join(h.root, "nope.yaml")}, aerr.StateIO},
		{"validate missing file", "", []string{"validate", "CHG-001", filepath.Join(h.root, "nope.yaml")}, aerr.StateIO},
	}
	for _, tt := range tests {
		h.stdin = tt.stdin
		if code, env := h.runJSON(append([]string{"spec"}, tt.args...)...); code == 0 || errCode(env) != string(tt.code) {
			t.Errorf("%s: %v", tt.name, env)
		}
	}
	dir := filepath.Join(h.root, ".athel", "changes", "CHG-001", "spec")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if l := h.mustOK("spec", "list", "CHG-001"); len(l["specs"].([]any)) != 0 {
		t.Errorf("only spec files count: %v", l)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.md"), []byte("# no block\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("spec", "list", "CHG-001"); code == 0 || errCode(env) != string(aerr.StateSchema) {
		t.Errorf("a broken spec file: %v", env)
	}
	delete(h.env, "ATHEL_SESSION_ID")
	h.stdin = specBlock
	if code, env := h.runJSON("spec", "save", "CHG-001", "-"); code == 0 || errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %v", env)
	}
}
