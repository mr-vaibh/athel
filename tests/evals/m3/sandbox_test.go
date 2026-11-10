package m3

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
	"github.com/mr-vaibh/athel/tests/internal/m3docs"
)

func newM3Sandbox(t *testing.T) (*evalkit.Sandbox, error) {
	t.Helper()
	return evalkit.NewSandbox(t.TempDir(), buildTools(t))
}

// TestM3Sandbox takes a change to planned with the CLI alone, in the eval sandbox, and grades
// what collect reads back: the runner's paths, the decide prompt and the grader agree with the
// files the CLI really writes, without a model.
func TestM3Sandbox(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fixture and three binaries")
	}
	sb, err := newM3Sandbox(t)
	if err != nil {
		t.Fatal(err)
	}
	run := func(stdin string, args ...string) []byte {
		t.Helper()
		out, err := sb.Athel(stdin, append([]string{"--json", "--session", "eval-check"}, args...)...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	run("", "change", "new", "--title", "Eval check", "--prd-text", m3docs.PRD)
	run("", "prd", "index", change)
	run(m3docs.Review, "prd", "review", "save", change, "-")
	run(m3docs.Conventions, "conventions", "save", m3docs.Repo, "-", "--new")
	run(m3docs.Spec, "spec", "save", change, "-")
	for _, to := range []string{"investigating", "specified", "awaiting_decisions"} {
		run("", "state", "transition", change, "--to", to, "--by", "athel", "--reason", "eval check")
	}
	answerAsDeveloper(t, run)
	run(m3docs.Plan("svc", []string{"src/**"}, []string{"edit", "test"}), "plan", "set", change, "-")
	run("", "trace", change)
	run("", "state", "transition", change, "--to", "planned", "--by", "athel", "--reason", "eval check")

	a, err := collect(sb.WS)
	if err != nil {
		t.Fatal(err)
	}
	c := Case{Expect: Expect{RiskAtLeast: "LOW", ReviewMissing: []string{"acceptance_criteria"}, Ordering: []string{"internal"}}}
	g := GradeRun(c, a)
	if !g.Planned || !g.TraceComplete || !g.KindsOK || !g.ReviewOK || !strings.Contains(strings.Join(g.Problems, ";"), "no planned blast radius report") {
		t.Errorf("grade of the CLI's own files: %+v", g)
	}
}

const card = `rule: A1
step: spec
question: Which formats should the download offer?
context: The PRD says download.
options:
  - {key: a, label: PDF only, consequence: one renderer}
  - {key: b, label: PDF and HTML, consequence: two outputs to test}
recommendation: {option: b, reason: both are asked for}
default_if_unanswered: a
default_safe: true
blocks: [plan]
new_blocker: false
`

// answerAsDeveloper raises a card and answers it the way the runner does: the scripted prompt
// goes through the real UserPromptSubmit hook, and the CLI accepts exactly the granted answer.
func answerAsDeveloper(t *testing.T, run func(string, ...string) []byte) {
	t.Helper()
	run(card, "decide", "add", change, "-")
	run("", "decide", "present", change, "--step", "spec")
	prompt, err := decidePrompt(run("", "decide", "list", change, "--open"))
	if err != nil || prompt != "/athel:decide CHG-001 D-1=b" {
		t.Fatalf("decide prompt: %q %v", prompt, err)
	}
	in, _ := json.Marshal(map[string]string{"session_id": "eval-check", "prompt_id": "p-1", "prompt": prompt})
	if out := run(string(in), "hook", "user-prompt-submit"); !strings.Contains(string(out), "athel decide answer CHG-001 'D-1=b'") {
		t.Fatalf("the hook names the granted command: %s", out)
	}
	run("", "decide", "answer", change, "D-1=b")
	if p, err := decidePrompt(run("", "decide", "list", change, "--open")); err != nil || p != "" {
		t.Errorf("every card answered, no decide prompt: %q %v", p, err)
	}
}

func TestDecidePrompt(t *testing.T) {
	list := `{"data":{"decisions":[{"id":"D-1","kind":"card","recommendation":{"option":"b"}},{"id":"D-2","kind":"decision"},{"id":"D-3","kind":"card","recommendation":{"option":"a"}}]}}`
	if p, err := decidePrompt([]byte(list)); err != nil || p != "/athel:decide CHG-001 D-1=b D-3=a" {
		t.Errorf("%q %v", p, err)
	}
	for _, bad := range []string{"{", `{"data":{"decisions":[{"id":"D-1","kind":"card"}]}}`} {
		if _, err := decidePrompt([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func TestCollectMissing(t *testing.T) {
	ws := t.TempDir()
	if a, err := collect(ws); err != nil || a != (Artifacts{}) {
		t.Errorf("an empty workspace has no artifacts: %+v %v", a, err)
	}
	dir := filepath.Join(ws, ".athel", "changes", change)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.yaml"), []byte("state: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := collect(ws); err == nil {
		t.Error("a broken change.yaml must fail")
	}
}

func TestSteps(t *testing.T) {
	var names []string
	for _, s := range steps("/tmp/prd.md") {
		names = append(names, s.name)
	}
	if strings.Join(names, ",") != "start,spec,decide,plan" {
		t.Errorf("steps: %v", names)
	}
	if p, _ := steps("/x/prd.md")[0].prompt(nil); p != "/athel:start /x/prd.md" {
		t.Errorf("start prompt: %q", p)
	}
}
