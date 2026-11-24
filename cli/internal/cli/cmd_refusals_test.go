package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refusals lists, per command, how many arguments it takes and what it answers in the three
// ways a call most often goes wrong: outside a workspace, on a change that does not exist (with
// a session), and with no session. "ok" is success; anything else is the typed error code. Every
// command must answer with one of these, never crash and never succeed silently where it should
// not. Commands that need flags to be called at all have their own tests.
const refusals = `
ask show|1|E-STATE-NOT-INITIALISED|E-STATE-INVALID|E-STATE-INVALID
audit tail|0|E-STATE-NOT-INITIALISED|ok|ok
audit verify|0|E-STATE-NOT-INITIALISED|ok|ok
blast score|1|E-STATE-IO|E-STATE-IO|E-STATE-IO
blast signoff|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
blast template|0|ok|ok|ok
budget add|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
budget show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
build context|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
build done|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
build next|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
build save|3|E-STATE-IO|E-STATE-IO|E-STATE-IO
build template|0|ok|ok|ok
change list|0|E-STATE-NOT-INITIALISED|ok|ok
change show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
checkpoint set|2|E-STATE-NO-SESSION|E-STATE-INVALID|E-STATE-NO-SESSION
checkpoint show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
config show|0|ok|ok|ok
config validate|0|ok|ok|ok
conventions save|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
conventions show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
decide add|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
decide list|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
decide record|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
decide show|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
decide template|0|ok|ok|ok
decide tick|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
fact add|1|E-STATE-IO|E-STATE-IO|E-STATE-IO
fact archive|0|E-STATE-NOT-INITIALISED|ok|ok
fact find|0|E-STATE-NOT-INITIALISED|ok|ok
fact show|1|E-STATE-NOT-INITIALISED|E-STATE-INVALID|E-STATE-INVALID
fact use|1|E-STATE-NOT-INITIALISED|E-STATE-INVALID|E-STATE-INVALID
fact verify|1|E-STATE-NOT-INITIALISED|E-STATE-INVALID|E-STATE-INVALID
finding add|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
finding fix|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
finding list|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
finding template|0|ok|ok|ok
gate baseline|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
gate run|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
gate show|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
gate waive|3|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
git lint|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
git preflight|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
git push|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
git worktree|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
init|0|ok|ok|ok
investigation template|0|ok|ok|ok
lock acquire|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
lock refresh|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
lock release|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
lock status|0|E-STATE-NOT-INITIALISED|ok|ok
migrate|0|E-STATE-NOT-INITIALISED|ok|ok
plan set|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
plan show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
plan summary|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
plan template|0|ok|ok|ok
playbook list|0|ok|ok|ok
playbook show|1|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
policy show|0|ok|ok|ok
policy validate|0|ok|ok|ok
pr body|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
pr open|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
pr ready|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
pr update|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
prd index|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
prd review save|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
prd review template|0|ok|ok|ok
prd scan|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
prd show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
qa save|3|E-STATE-IO|E-STATE-IO|E-STATE-IO
qa template|0|ok|ok|ok
repos cache|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
repos index|0|E-STATE-NOT-INITIALISED|E-STATE-INVALID|E-STATE-INVALID
repos roots propose|0|E-STATE-IO|E-STATE-IO|E-STATE-IO
repos roots set|1|E-STATE-NO-SESSION|E-POLICY-HUMAN-ORIGIN|E-STATE-NO-SESSION
repos show|0|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
repos survey|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
resume|0|E-STATE-NO-SESSION|ok|E-STATE-NO-SESSION
review context|2|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
review dispute|3|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
review fix|3|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
review rules|0|ok|ok|ok
review save|3|E-STATE-IO|E-STATE-IO|E-STATE-IO
review show|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
review template|0|ok|ok|ok
schema validate|1|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
security save|3|E-STATE-IO|E-STATE-IO|E-STATE-IO
security template|0|ok|ok|ok
set show|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
spec list|1|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
spec save|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
spec show|2|E-STATE-NOT-INITIALISED|E-STATE-NOT-FOUND|E-STATE-NOT-FOUND
spec template|0|ok|ok|ok
spec validate|2|E-STATE-IO|E-STATE-IO|E-STATE-IO
state enter|2|E-STATE-NO-SESSION|E-STATE-INVALID|E-STATE-NO-SESSION
state exit|2|E-STATE-NO-SESSION|E-STATE-INVALID|E-STATE-NO-SESSION
state show|0|E-STATE-NOT-INITIALISED|ok|ok
trace|1|E-STATE-NO-SESSION|E-STATE-NOT-FOUND|E-STATE-NO-SESSION
version|0|ok|ok|ok
`

func TestEveryCommandRefusesCleanly(t *testing.T) {
	inside := setup(t)
	nosess := setup(t)
	delete(nosess.env, "ATHEL_SESSION_ID")
	dummy := []string{"CHG-404", "T-01", "x"}
	seen := map[string]bool{}
	for _, row := range strings.Split(strings.TrimSpace(refusals), "\n") {
		f := strings.Split(row, "|")
		name, k := f[0], int(f[1][0]-'0')
		seen[name] = true
		outside := newHarness(t) // fresh for each row: an earlier "init" must not make it a workspace
		for i, h := range []*harness{outside, inside, nosess} {
			args := append(append([]string{"--json"}, strings.Fields(name)...), dummy[:k]...)
			code, out, _ := h.run(args...)
			var env map[string]any
			if err := json.Unmarshal([]byte(out), &env); err != nil && code != 0 {
				t.Errorf("%s (case %d): exit %d with no JSON envelope: %q", name, i, code, out)
				continue
			}
			got := "ok"
			if e, ok := env["error"].(map[string]any); ok {
				got, _ = e["code"].(string)
			}
			if code != 0 && got == "ok" {
				got = "exit " + itoa(code)
			}
			if got != f[2+i] {
				t.Errorf("%s (case %d): %s, want %s", name, i, got, f[2+i])
			}
		}
	}
	for name := range commands() {
		if !seen[name] && !needsFlags[name] {
			t.Errorf("command %q has no row in refusals: add one", name)
		}
	}
}

// needsFlags are commands that cannot run without flags; their tests live with them.
var needsFlags = map[string]bool{
	"ask new": true, "blast check": true, "blast diff": true, "blast save": true, "change new": true,
	"decide answer": true, "decide present": true, "git hook": true, "hook": true, "integrity accept": true,
	"investigation save": true, "finding triage": true, "investigation validate": true, "plan diff": true, "policy decide": true,
	"prd add-version": true, "prd diff": true, "set coordinate": true, "spec remove": true, "state transition": true,
}

// TestPreToolUseWithoutCwd: a tool call with no folder is judged in the session's own folder.
func TestPreToolUseWithoutCwd(t *testing.T) {
	h := setup(t)
	h.stdin = `{"session_id": "sess-A", "tool_name": "Bash", "tool_input": {"command": "cat .env"}}` // no cwd
	out := hookOut(t, h, "pre-tool-use")
	if !strings.Contains(asJSON(t, out), "deny") {
		t.Errorf("a secret read is denied wherever it runs: %v", out)
	}
}

// TestBrokenSignoffBlocksApproval: an unreadable sign-off never counts as one.
func TestBrokenSignoffBlocksApproval(t *testing.T) {
	h := setup(t)
	h.mustOK("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	m3Spec(t, h)
	h.stdin = plannedBlast() // CRITICAL: approval needs a sign-off
	h.mustOK("blast", "save", "--change", "CHG-001", "-")
	h.stdin = ""
	m3Plan(t, h)
	for _, to := range []string{"specified", "awaiting_decisions", "planned"} {
		h.mustOK("state", "transition", "CHG-001", "--to", to, "--by", "athel", "--reason", "r")
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "blast-signoff.yaml"), []byte("schema: nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.humanPrompt("/athel:approve CHG-001")
	if code, env := h.runJSON("state", "transition", "CHG-001", "--to", "plan_approved", "--by", "human", "--reason", "ok"); code == 0 {
		t.Errorf("approved with a broken sign-off: %v", env)
	}
}

// TestPRDIndexWithoutThePRD: a change whose PRD file is gone cannot be indexed.
func TestPRDIndexWithoutThePRD(t *testing.T) {
	h := setup(t)
	dir := filepath.Join(h.root, ".athel", "changes", "CHG-001", "prd")
	for _, f := range []string{"v1.md", "v1.index.yaml"} {
		if err := os.Remove(filepath.Join(dir, f)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if _, env := h.runJSON("prd", "index", "CHG-001"); errCode(env) != "E-STATE-IO" {
		t.Errorf("%v", env)
	}
}
