package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const prdV1 = "# Phone\n- Customers must change their phone number\n- Updates must be fast, within 200 ms\n"

func prdSetup(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.mustOK("init")
	h.mustOK("change", "new", "--title", "Phone", "--prd-text", prdV1)
	h.env["ATHEL_SESSION_ID"] = "sess-A"
	return h
}

func items(d map[string]any) []string {
	var out []string
	list, _ := d["items"].([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		out = append(out, m["id"].(string)+" "+m["text"].(string))
	}
	return out
}

func TestChangeNewIndexesThePRD(t *testing.T) {
	h := prdSetup(t)
	got := strings.Join(items(h.mustOK("prd", "show", "CHG-001")), "; ")
	if got != "PRD-1 Phone; PRD-1.1 Customers must change their phone number; PRD-1.2 Updates must be fast, within 200 ms" {
		t.Errorf("index: %s", got)
	}
	if again := strings.Join(items(h.mustOK("prd", "index", "CHG-001")), "; "); again != got {
		t.Errorf("index must never renumber: %s", again)
	}
	scan := h.mustOK("prd", "scan", "CHG-001")
	if s := strings.Join([]string{asJSON(t, scan["ambiguous"]), asJSON(t, scan["limits"])}, " "); !strings.Contains(s, `"phrase":"fast"`) || !strings.Contains(s, `"phrase":"200 ms"`) {
		t.Errorf("scan: %s", s)
	}
}

func TestPRDIndexForOlderChanges(t *testing.T) {
	h := prdSetup(t)
	if err := os.Remove(filepath.Join(h.root, ".athel", "changes", "CHG-001", "prd", "v1.index.yaml")); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("prd", "show", "CHG-001"); code == 0 || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("no index yet: %d %v", code, env)
	}
	if got := items(h.mustOK("prd", "index", "CHG-001")); len(got) != 3 {
		t.Errorf("rebuilt: %v", got)
	}
}

func TestPRDAddVersionAndDiff(t *testing.T) {
	h := prdSetup(t)
	h.stdin = "# Phone\n- Customers must change their phone number\n- Updates must finish within 200 ms\n"
	added := h.mustOK("prd", "add-version", "CHG-001", "--prd-stdin")
	if added["version"] != "v2" || !strings.Contains(asJSON(t, added["diff"]), `"id":"PRD-1.3"`) || !strings.Contains(asJSON(t, added["diff"]), `"removed":[{"id":"PRD-1.2"`) {
		t.Errorf("added: %v", added)
	}
	if d := h.mustOK("prd", "diff", "CHG-001", "--from", "v1"); !strings.Contains(asJSON(t, d), `"added":[{"id":"PRD-1.3"`) {
		t.Errorf("diff: %v", d)
	}
	if code, _ := h.runJSON("prd", "diff", "CHG-001"); code != 2 {
		t.Errorf("--from is required: %d", code)
	}
	if code, env := h.runJSON("prd", "diff", "CHG-001", "--from", "v9"); code == 0 || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("unknown version: %v", env)
	}
}

const reviewBlock = `schema: athel/prd-review@1
change: CHG-001
version: v1
checklist:
  goal: {status: present, prd: [PRD-1.1]}
  actors: {status: present, prd: [PRD-1.1]}
  scope: {status: present, prd: [PRD-1]}
  out_of_scope: {status: missing, prd: []}
  functional_requirements: {status: present, prd: [PRD-1.1]}
  acceptance_criteria: {status: missing, prd: []}
  non_functional: {status: partial, prd: [PRD-1.2]}
  constraints: {status: missing, prd: []}
  dependencies: {status: missing, prd: []}
ambiguities:
  - {prd: PRD-1.2, phrase: fast, material: true, resolution: card, ref: D-1}
contradictions: []
assumptions: []
`

func TestPRDReviewSave(t *testing.T) {
	h := prdSetup(t)
	h.stdin = reviewBlock
	saved := h.mustOK("prd", "review", "save", "CHG-001", "-")
	if !strings.HasSuffix(saved["path"].(string), "review-v1.md") || !strings.Contains(asJSON(t, saved["missing"]), "acceptance_criteria (missing)") {
		t.Errorf("saved: %v", saved)
	}
	b, err := os.ReadFile(saved["path"].(string))
	if err != nil || !strings.Contains(string(b), "```yaml\nschema: athel/prd-review@1") {
		t.Errorf("file: %s %v", b, err)
	}
	h.stdin = strings.Replace(reviewBlock, "prd: [PRD-1.1]}", "prd: [PRD-4.4]}", 1)
	if code, env := h.runJSON("prd", "review", "save", "CHG-001", "-"); code == 0 || errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("unknown PRD ID: %v", env)
	}
	h.stdin = "schema: nope\n"
	if code, env := h.runJSON("prd", "review", "save", "CHG-001", "-"); code == 0 || errCode(env) != string(aerr.StateSchema) {
		t.Errorf("bad block: %v", env)
	}
	if code, env := h.runJSON("prd", "review", "save", "CHG-001", filepath.Join(h.root, "missing.yaml")); code == 0 || errCode(env) != string(aerr.StateIO) {
		t.Errorf("missing file: %v", env)
	}
	file := filepath.Join(t.TempDir(), "r.yaml")
	if err := os.WriteFile(file, []byte(reviewBlock), 0o600); err != nil {
		t.Fatal(err)
	}
	h.stdin = ""
	h.mustOK("prd", "review", "save", "CHG-001", file)
	delete(h.env, "ATHEL_SESSION_ID")
	if code, env := h.runJSON("prd", "review", "save", "CHG-001", file); code == 0 || errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %v", env)
	}
}

func TestPRDCommandErrors(t *testing.T) {
	h := prdSetup(t)
	for _, args := range [][]string{{"prd", "show", "CHG-009"}, {"prd", "scan", "CHG-009"}, {"prd", "index", "CHG-009"}, {"prd", "add-version", "CHG-009", "--prd-text", "x"}} {
		if code, env := h.runJSON(args...); code == 0 || errCode(env) != string(aerr.StateNotFound) {
			t.Errorf("%v: %v", args, env)
		}
	}
	if code, _ := h.runJSON("prd", "add-version", "CHG-001"); code != 2 {
		t.Errorf("no PRD given: %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "prd", "v1.index.yaml"), []byte("bogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("prd", "show", "CHG-001"); code == 0 || errCode(env) != string(aerr.StateSchema) {
		t.Errorf("corrupt index: %v", env)
	}
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
