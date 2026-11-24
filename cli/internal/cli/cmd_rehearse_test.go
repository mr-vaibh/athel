package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/changeset"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// rehearsalSet makes the goGates change a two-repo set, users then orders. The fixture has one
// remote, so orders is a second view of it: the rehearsal clones, merges and checks it on its
// own all the same.
func rehearsalSet(t *testing.T, h *harness) {
	t.Helper()
	s := state.NewStore(state.Workspace{Root: h.root}, schema.NewRegistry(), h.clock)
	if _, err := s.Update("CHG-001", func(c *state.Change) error {
		users, _ := repoWork(c, "users")
		orders := users
		orders.Name = "orders"
		c.Repos = append([]state.RepoWork{users}, orders)
		c.ChangeSet = &changeset.Set{Strategy: changeset.ExpandContract, Invariant: changeset.Invariant, Repos: []changeset.Entry{
			{Repo: "users", Role: changeset.Provider, Order: 1, DependsOn: []string{}},
			{Repo: "orders", Role: changeset.Consumer, Order: 2, DependsOn: []string{"users"}},
		}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func systemCheck(t *testing.T, h *harness, argv string) {
	t.Helper()
	var words []string
	for _, w := range strings.Fields(argv) {
		words = append(words, `"`+w+`"`)
	}
	doc := "schema: athel/system-check@1\ncommand: [" + strings.Join(words, ", ") + "]\n"
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "system.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRehearsalCatchesBrokenStep (ADR 036): each state after a merge is rebuilt and checked; a
// failing system check fails the rehearsal, and pr ready waits for a passing, current one.
func TestRehearsalCatchesBrokenStep(t *testing.T) {
	h, tree := goGates(t)
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	s := state.NewStore(state.Workspace{Root: h.root}, schema.NewRegistry(), h.clock)
	if _, err := s.Update("CHG-001", func(c *state.Change) error { c.ChangeSet = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("set", "verify", "CHG-001"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "no change set") {
		t.Errorf("verify without a set: %v", env)
	}
	rehearsalSet(t, h)
	if code, env := h.runJSON("set", "verify", "CHG-001"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "not pushed yet") {
		t.Errorf("verify before the push: %v", env)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	rehearsalSet(t, h) // orders copies the pushed users entry

	v := h.mustOK("set", "verify", "CHG-001")
	steps := v["steps"].([]any)
	if v["result"] != "pass" || len(steps) != 2 || steps[0].(map[string]any)["system"].(map[string]any)["status"] != "not_configured" {
		t.Fatalf("no system check: %v", v)
	}
	first := asJSON(t, steps[0])
	if !strings.Contains(first, `"gate":"build","repo":"users","status":"pass"`) || !strings.Contains(first, `"gate":"unit","repo":"users","status":"pass"`) {
		t.Errorf("users' gates at step 1: %s", first)
	}

}

// TestRehearsalFailsOnTheSystemCheck: a failing system check fails the rehearsal; the result is
// recorded in the set and the evidence, and the throwaway clones are gone.
func TestRehearsalFailsOnTheSystemCheck(t *testing.T) {
	h, tree := goGates(t)
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	rehearsalSet(t, h)
	systemCheck(t, h, "go -C {orders} run ./no-such-command")
	v := h.mustOK("set", "verify", "CHG-001")
	steps := v["steps"].([]any)
	if v["result"] != "fail" || steps[1].(map[string]any)["merged"] == nil {
		t.Errorf("a failing system check fails the rehearsal: %v", v)
	}
	if _, err := os.Stat(filepath.Join(h.root, "cache", "rehearsal", "CHG-001")); !os.IsNotExist(err) {
		t.Errorf("the throwaway clones are removed: %v", err)
	}
	set := h.mustOK("set", "show", "CHG-001")["set"].(map[string]any)
	if set["verify"].(map[string]any)["result"] != "fail" {
		t.Errorf("the set records the result: %v", set["verify"])
	}
	if b, err := os.ReadFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "changeset", "verify.yaml")); err != nil || !strings.Contains(string(b), "status: fail") {
		t.Errorf("evidence: %v %s", err, b)
	}
}

func TestRehearsalGatesPRReady(t *testing.T) {
	h, tree := goGates(t)
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	recordingGH(t, h)
	h.mustOK("git", "push", "CHG-001", "users")
	h.mustOK("pr", "open", "CHG-001", "users")
	rehearsalSet(t, h)
	s := state.NewStore(state.Workspace{Root: h.root}, schema.NewRegistry(), h.clock)
	if _, err := s.Update("CHG-001", func(c *state.Change) error { c.State = state.ReadyForPR; return nil }); err != nil {
		t.Fatal(err)
	}
	ready := func(want string) {
		t.Helper()
		code, env := h.runJSON("pr", "ready", "CHG-001", "users")
		if want == "" && code != 0 || want != "" && !strings.Contains(asJSON(t, env), want) {
			t.Errorf("pr ready: %d %v, want %q", code, env, want)
		}
	}
	ready("has not been rehearsed")
	systemCheck(t, h, "go -C {orders} run ./no-such-command")
	h.mustOK("set", "verify", "CHG-001")
	ready("rehearsal of CHG-001 failed")
	systemCheck(t, h, "go -C {orders} vet ./...")
	h.mustOK("set", "verify", "CHG-001")
	ready("")
	if out, err := commit(t, h, tree, "src/c.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	ready("is stale: ")
}

func TestPlaceholders(t *testing.T) {
	got := placeholders([]string{"shopsys", "--users", "{users}", "--orders", "{orders}", "{users}", "{}", "x{y}"})
	if strings.Join(got, ",") != "users,orders" {
		t.Errorf("placeholders = %v", got)
	}
}

// TestRehearsalNeverPushes: the rehearsal merges in throwaway clones of the cache; the remote
// keeps exactly the branches it had. A system check Athel's policy forbids never runs.
func TestRehearsalNeverPushes(t *testing.T) {
	h, tree := goGates(t)
	if out, err := commit(t, h, tree, "src/b.go", "package src\n", goodMsg); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	h.mustOK("git", "push", "CHG-001", "users")
	rehearsalSet(t, h)
	remote := func() string {
		cmd := exec.Command("git", "-C", tree, "ls-remote", "origin")
		cmd.Env = append(os.Environ(), "HOME="+h.env["HOME"]) // its .gitconfig maps github.com/acme to the local remotes
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v %s", err, out)
		}
		return string(out)
	}
	before := remote()
	systemCheck(t, h, "curl -s https://example.invalid/{orders}")
	v := h.mustOK("set", "verify", "CHG-001")
	sys := v["steps"].([]any)[0].(map[string]any)["system"].(map[string]any)
	if v["result"] != "fail" || sys["status"] != "unavailable" || !strings.Contains(asJSON(t, sys), "policy does not let the CLI run it") {
		t.Errorf("a forbidden system check: %v", sys)
	}
	if after := remote(); after != before {
		t.Errorf("the remote changed:\n%s\n%s", before, after)
	}
}
