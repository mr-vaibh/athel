package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// TestConsumerManifestHasProviderDiff (M5-plan G10, A2): the consumer task in orders gets the
// patch of the users task it waits for, from Athel's users worktree; a no-code task has none.
func TestConsumerManifestHasProviderDiff(t *testing.T) {
	p := &plan.Plan{Tasks: []plan.Task{
		{ID: "T-01", Title: "Serve v2", Repo: "users", Kind: plan.KindExpand, Paths: []string{"api/**"}},
		{ID: "T-02", Title: "Ask the gateway team", Repo: "gateway", Kind: plan.KindInternal},
		{ID: "T-03", Title: "Call v2", Repo: "orders", Kind: plan.KindConsume, DependsOn: []string{"T-01", "T-02"}},
	}}
	c := &state.Change{
		Repos: []state.RepoWork{{Name: "users", Worktree: "repos/users@CHG-001"}, {Name: "orders", Worktree: "repos/orders@CHG-001"}},
		Build: []build.Task{{ID: "T-01", Status: build.Done, Commits: []string{"aaa", "bbb"}}, {ID: "T-02", Status: build.Done}},
	}
	var shown []string
	saved := map[string]string{}
	show := func(worktree string, commits []string) (string, error) {
		shown = append(shown, worktree+" "+strings.Join(commits, ","))
		return "+\tmux.HandleFunc(\"GET /v2/users/{id}\", getUser)\n", nil
	}
	save := func(name string, b []byte) error { saved[name] = string(b); return nil }
	got, err := upstreamOf(p, c, "T-03", show, save)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (upstreamTask{Task: "T-01", Repo: "users", Kind: "expand", Title: "Serve v2", Patch: "build/T-03/upstream-T-01.patch"}) {
		t.Errorf("upstream = %+v", got)
	}
	if strings.Join(shown, ";") != "repos/users@CHG-001 aaa,bbb" || !strings.Contains(saved["build/T-03/upstream-T-01.patch"], "/v2/users/") {
		t.Errorf("shown %q saved %v", shown, saved)
	}
	if got, _ := upstreamOf(p, c, "T-01", show, save); len(got) != 0 {
		t.Errorf("a provider waits for nothing: %+v", got)
	}
}

func TestUpstreamErrors(t *testing.T) {
	p := &plan.Plan{Tasks: []plan.Task{{ID: "T-01", Repo: "users", Paths: []string{"api/**"}}, {ID: "T-02", Repo: "orders", DependsOn: []string{"T-01"}}}}
	c := &state.Change{Repos: []state.RepoWork{{Name: "users", Worktree: "w"}}, Build: []build.Task{{ID: "T-01", Commits: []string{"a"}}}}
	boom := errors.New("boom")
	ok := func(string, []string) (string, error) { return "x", nil }
	if _, err := upstreamOf(p, c, "T-02", func(string, []string) (string, error) { return "", boom }, nil); !errors.Is(err, boom) {
		t.Errorf("git failure: %v", err)
	}
	if _, err := upstreamOf(p, c, "T-02", ok, func(string, []byte) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("save failure: %v", err)
	}
	c.Repos = nil
	if got, err := upstreamOf(p, c, "T-02", ok, nil); err != nil || len(got) != 0 {
		t.Errorf("no worktree, no patch: %v %v", got, err)
	}
}
