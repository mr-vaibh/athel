package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const cardYAML = `rule: A1
step: spec
question: Which phone formats should the API accept?
context: PRD-2.1 says "valid phone number".
options:
  - {key: a, label: E.164 only, consequence: Clean data.}
  - {key: b, label: E.164 plus local formats, consequence: Friendlier.}
recommendation: {option: b, reason: the app sends local formats}
default_if_unanswered: b
default_safe: true
blocks: [T-03]
`

func TestDecideAddRecordListShow(t *testing.T) {
	h := prdSetup(t)
	h.stdin = cardYAML
	if added := h.mustOK("decide", "add", "CHG-001", "-"); added["id"] != "D-1" || added["kind"] != "card" || added["answer"] != nil {
		t.Fatalf("add: %v", added)
	}
	h.stdin = "step: plan\nquestion: Where does the validator live?\nchoice: src/validation\nreason: conventions\n"
	if rec := h.mustOK("decide", "record", "CHG-001", "-"); rec["id"] != "D-2" || rec["decided_by"] != "athel" || rec["at"] != "2026-11-01T10:00:00.000Z" {
		t.Errorf("record: %v", rec)
	}
	for _, tt := range []struct {
		args        []string
		has, hasNot string
	}{
		{[]string{"list", "CHG-001", "--open"}, `"id":"D-1"`, `"id":"D-2"`},
		{[]string{"list", "CHG-001", "--step", "plan"}, `"id":"D-2"`, `"id":"D-1"`},
		{[]string{"show", "CHG-001", "D-2"}, `"choice":"src/validation"`, `"rule"`},
	} {
		got := asJSON(t, h.mustOK(append([]string{"decide"}, tt.args...)...))
		if !strings.Contains(got, tt.has) || strings.Contains(got, tt.hasNot) {
			t.Errorf("%v: %s", tt.args, got)
		}
	}
}

func TestDecidePresentOnce(t *testing.T) {
	h := prdSetup(t)
	h.stdin = cardYAML
	h.mustOK("decide", "add", "CHG-001", "-")
	if batch := h.mustOK("decide", "present", "CHG-001", "--step", "spec"); !strings.Contains(asJSON(t, batch), `"presented_at":"2026-11-01T10:00:00.000Z"`) {
		t.Errorf("present: %v", batch)
	}
	h.stdin = strings.Replace(cardYAML, "Which phone formats should the API accept?", "Keep the old field?", 1)
	h.mustOK("decide", "add", "CHG-001", "-")
	if code, env := h.runJSON("decide", "present", "CHG-001", "--step", "spec"); code == 0 || errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("a second interruption: %v", env)
	}
	if b, err := os.ReadFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "decisions.yaml")); err != nil || !strings.Contains(string(b), "schema: athel/decision@1") {
		t.Errorf("file: %s %v", b, err)
	}
}

func TestDecideErrors(t *testing.T) {
	h := prdSetup(t)
	missing := filepath.Join(h.root, "missing.yaml")
	tests := []struct {
		name, stdin string
		args        []string
		code        aerr.Code
	}{
		{"a card may not bring its own answer", cardYAML + "answer: {option: b, by: human, at: x}\n", []string{"add", "CHG-001", "-"}, aerr.StateSchema},
		{"a card may not bring its own ID", "id: D-7\n" + cardYAML, []string{"add", "CHG-001", "-"}, aerr.StateSchema},
		{"lint", strings.Replace(cardYAML, "default_if_unanswered: b", "default_if_unanswered: d", 1), []string{"add", "CHG-001", "-"}, aerr.StateInvalid},
		{"record unknown field", "step: plan\nbogus: 1\n", []string{"record", "CHG-001", "-"}, aerr.StateSchema},
		{"record without choice and reason", "step: plan\nquestion: q\n", []string{"record", "CHG-001", "-"}, aerr.StateSchema},
		{"show unknown", "", []string{"show", "CHG-001", "D-9"}, aerr.StateNotFound},
		{"list unknown change", "", []string{"list", "CHG-009"}, aerr.StateNotFound},
		{"add to unknown change", cardYAML, []string{"add", "CHG-009", "-"}, aerr.StateNotFound},
		{"add from a missing file", "", []string{"add", "CHG-001", missing}, aerr.StateIO},
		{"record from a missing file", "", []string{"record", "CHG-001", missing}, aerr.StateIO},
	}
	for _, tt := range tests {
		h.stdin = tt.stdin
		if code, env := h.runJSON(append([]string{"decide"}, tt.args...)...); code == 0 || errCode(env) != string(tt.code) {
			t.Errorf("%s: %v", tt.name, env)
		}
	}
	if code, _ := h.runJSON("decide", "present", "CHG-001"); code != 2 {
		t.Errorf("--step is required: %d", code)
	}
}

func TestDecideCorruptFileAndNoSession(t *testing.T) {
	h := prdSetup(t)
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "decisions.yaml"), []byte("schema: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.stdin = cardYAML
	for _, args := range [][]string{{"decide", "list", "CHG-001"}, {"decide", "add", "CHG-001", "-"}} {
		if code, env := h.runJSON(args...); code == 0 || errCode(env) != string(aerr.StateSchema) {
			t.Errorf("corrupt file %v: %v", args, env)
		}
	}
	delete(h.env, "ATHEL_SESSION_ID")
	if code, env := h.runJSON("decide", "add", "CHG-001", "-"); code == 0 || errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %v", env)
	}
}

// decideSetup raises two cards and presents them, as the spec step would.
func decideSetup(t *testing.T) *harness {
	t.Helper()
	h := prdSetup(t)
	for _, q := range []string{"Which phone formats should the API accept?", "Keep the old field?"} {
		h.stdin = strings.Replace(cardYAML, "Which phone formats should the API accept?", q, 1)
		h.mustOK("decide", "add", "CHG-001", "-")
	}
	h.stdin = ""
	h.mustOK("decide", "present", "CHG-001", "--step", "spec")
	return h
}

func TestDecideAnswerNeedsTheDevelopersGrant(t *testing.T) {
	h := decideSetup(t)
	if code, env := h.runJSON("decide", "answer", "CHG-001", "D-1=a"); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyHumanOrigin) {
		t.Fatalf("answer without the developer's prompt: %d %v", code, env)
	}
	h.humanPrompt("/athel:decide CHG-001 D-1=a D-2='keep it for one release' skip=D-2")
	if code, _ := h.runJSON("decide", "answer", "CHG-001", "D-1=b", "D-2='keep it for one release'", "skip=D-2"); code != aerr.ExitPolicy {
		t.Errorf("an answer other than the developer's: exit %d", code)
	}
	got := h.mustOK("decide", "answer", "CHG-001", "skip=D-2", "D-1=a", "D-2='keep it for one release'")
	if got["grant"] == "" || !strings.Contains(asJSON(t, got["answers"]), `"option":"a"`) {
		t.Errorf("answer: %v", got)
	}
	d1, d2 := h.mustOK("decide", "show", "CHG-001", "D-1"), h.mustOK("decide", "show", "CHG-001", "D-2")
	if !strings.Contains(asJSON(t, d1["answer"]), `"by":"human"`) || !strings.Contains(asJSON(t, d2["answer"]), `"checked":false`) || d2["should_not_have_asked"] != true {
		t.Errorf("recorded: %v / %v", d1, d2)
	}
	if code, _ := h.runJSON("decide", "answer", "CHG-001", "D-1=a"); code != aerr.ExitPolicy {
		t.Errorf("a grant is used once: exit %d", code)
	}
}

func TestDecideAnswerErrors(t *testing.T) {
	h := decideSetup(t)
	for _, args := range [][]string{{"decide", "answer", "CHG-001"}, {"decide", "answer", "CHG-001", "D-1=z"}, {"decide", "answer", "CHG-001", "please"}} {
		if code, _ := h.runJSON(args...); code != 2 {
			t.Errorf("%v: exit %d, want a usage error", args, code)
		}
	}
	h.humanPrompt("/athel:decide CHG-001 D-9=a")
	if code, env := h.runJSON("decide", "answer", "CHG-001", "D-9=a"); code == 0 || errCode(env) != string(aerr.StateInvalid) {
		t.Errorf("unknown card: %v", env)
	}
	delete(h.env, "ATHEL_SESSION_ID")
	if code, env := h.runJSON("decide", "answer", "CHG-001", "D-1=a"); code == 0 || errCode(env) != string(aerr.StateNoSession) {
		t.Errorf("no session: %v", env)
	}
}

func TestDecideTick(t *testing.T) {
	h := decideSetup(t)
	if got := h.mustOK("decide", "tick", "CHG-001"); asJSON(t, got) != `{"applied_defaults":[],"remind":[]}` {
		t.Errorf("at once: %v", got)
	}
	h.clock.Advance(4 * time.Hour)
	if got := h.mustOK("decide", "tick", "CHG-001"); asJSON(t, got) != `{"applied_defaults":["D-1","D-2"],"remind":[]}` {
		t.Errorf("after 4 h: %v", got)
	}
	if d := h.mustOK("decide", "show", "CHG-001", "D-1"); !strings.Contains(asJSON(t, d["answer"]), `"by":"default"`) {
		t.Errorf("default applied: %v", d)
	}
}
