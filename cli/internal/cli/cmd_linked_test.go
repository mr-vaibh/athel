package cli

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/changeset"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// asSet makes the harness's change a two-repo set: users then orders, whose PR acme/orders#777 is
// already open. orders borrows the users worktree: the body only needs a diff to count.
func asSet(t *testing.T, h *harness) {
	t.Helper()
	s := state.NewStore(state.Workspace{Root: h.root}, schema.NewRegistry(), h.clock)
	url := "https://github.com/acme/orders/pull/777"
	_, err := s.Update("CHG-001", func(c *state.Change) error {
		c.Repos = append(c.Repos, state.RepoWork{Name: "orders", Remote: "github.com/acme/orders", Base: "main",
			BaseHead: strings.Repeat("b", 40), Branch: "athel/CHG-001-phone-update", Worktree: "repos/users@CHG-001", PR: &url})
		c.ChangeSet = &changeset.Set{Strategy: changeset.ExpandContract, Invariant: changeset.Invariant, Repos: []changeset.Entry{
			{Repo: "users", Role: changeset.Provider, Order: 1, DependsOn: []string{}},
			{Repo: "orders", Role: changeset.Consumer, Order: 2, DependsOn: []string{"users"}},
		}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestOpeningOneRefreshesSiblings (FR-GIT-008, M5-plan G5): opening the users PR records it in
// the change set and rewrites the orders PR's body, which now names it and its place.
func TestOpeningOneRefreshesSiblings(t *testing.T) {
	h, _, _, _ := building(t)
	log := recordingGH(t, h)
	h.mustOK("git", "push", "CHG-001", "users")
	asSet(t, h)
	v := h.mustOK("pr", "open", "CHG-001", "users")
	if !strings.Contains(v["body"].(string), "This PR is step 1 of 2 in the merge order of CHG-001 (2 PRs).") ||
		!strings.Contains(v["body"].(string), "2. acme/orders#777 (consumer)") {
		t.Errorf("users body: %s", v["body"])
	}
	edit := ""
	for _, c := range calls(t, log) {
		if strings.HasPrefix(c, "pr edit 777 --repo acme/orders --body") {
			edit = c
		}
	}
	if !strings.Contains(edit, "Merge after acme/users#901.") || !strings.Contains(edit, "1. acme/users#901 (provider)") {
		t.Errorf("the orders PR is refreshed with the users PR: %q", edit)
	}
	set := h.mustOK("set", "show", "CHG-001")
	users := set["set"].(map[string]any)["repos"].([]any)[0].(map[string]any)
	if users["pr"] != "acme/users#901" || users["state"] != "draft" {
		t.Errorf("the set records the users PR: %v", set)
	}
}

func TestReadyOnlyTogether(t *testing.T) {
	c := &state.Change{ChangeSet: &changeset.Set{Repos: []changeset.Entry{{Repo: "users"}, {Repo: "orders"}, {Repo: "users", Deferred: true}}}}
	url := "https://github.com/acme/users/pull/901"
	c.Repos = []state.RepoWork{{Name: "users", PR: &url}, {Name: "orders"}}
	if got := unopened(c); strings.Join(got, ",") != "orders" {
		t.Errorf("unopened = %v", got)
	}
	if got := unopened(&state.Change{}); got != nil {
		t.Errorf("a change without a set: %v", got)
	}
}

func TestPRReadyRefusedBeforeEverySiblingOpens(t *testing.T) {
	h, _, _, _ := building(t)
	recordingGH(t, h)
	h.mustOK("git", "push", "CHG-001", "users")
	h.mustOK("pr", "open", "CHG-001", "users")
	s := state.NewStore(state.Workspace{Root: h.root}, schema.NewRegistry(), h.clock)
	if _, err := s.Update("CHG-001", func(c *state.Change) error {
		c.ChangeSet = &changeset.Set{Strategy: changeset.ExpandContract, Invariant: changeset.Invariant, Repos: []changeset.Entry{
			{Repo: "users", Role: changeset.Provider, Order: 1, DependsOn: []string{}},
			{Repo: "orders", Role: changeset.Consumer, Order: 2, DependsOn: []string{"users"}},
		}}
		c.State = state.ReadyForPR
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if code, env := h.runJSON("pr", "ready", "CHG-001", "users"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "marked ready together; not opened yet: orders") {
		t.Errorf("ready before orders opens: %v", env)
	}
}
