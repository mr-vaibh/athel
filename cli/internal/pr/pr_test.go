package pr

import (
	"strings"
	"testing"
)

var exclude = []string{"*.lock", "package-lock.json", "**/generated/**"}

func TestSize(t *testing.T) {
	numstat := "10\t2\tsrc/a.go\n" +
		"300\t0\tgo.sum.lock\n" +
		"5\t5\tweb/package-lock.json\n" +
		"40\t0\tapi/generated/client.go\n" +
		"-\t-\tassets/logo.png\n" +
		"3\t1\tsrc/b.go\n\n"
	got := Size(numstat, exclude)
	if got.Lines != 16 || strings.Join(got.Excluded, ",") != "go.sum.lock,web/package-lock.json,api/generated/client.go" || len(got.Files) != 3 {
		t.Errorf("%+v", got)
	}
	if got := Size("x\ty\tz\nbad line\n", nil); got.Lines != 0 || len(got.Files) != 1 {
		t.Errorf("unreadable counts are 0: %+v", got)
	}
}

func TestSplit(t *testing.T) {
	tasks := []TaskLines{{"T-01", 150}, {"T-02", 200}, {"T-03", 120}, {"T-04", 500}, {"T-05", 30}}
	got := Split(tasks, 400)
	want := []string{"T-01,T-02", "T-03", "T-04", "T-05"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, g := range got {
		if strings.Join(g.Tasks, ",") != want[i] {
			t.Errorf("PR %d = %v, want %s", i+1, g.Tasks, want[i])
		}
	}
	if !got[2].Oversized || got[0].Oversized || got[0].Lines != 350 {
		t.Errorf("a task over the limit alone is flagged: %+v", got)
	}
	if got := Split(nil, 400); len(got) != 0 {
		t.Errorf("no tasks: %+v", got)
	}
}

func TestTitle(t *testing.T) {
	if got := Title("users", "Phone update"); got != "feat(users): phone update" {
		t.Errorf("%q", got)
	}
	if got := Title("users", "  "); got != "feat(users): change" {
		t.Errorf("%q", got)
	}
}

func TestBody(t *testing.T) {
	b := Body(BodyInput{
		Change: "CHG-001", Title: "Phone update", Repo: "users", Spec: ".athel/changes/CHG-001/spec/users.md", PRD: "prd/v1.md",
		Files:     []File{{"src/a.go", 12}, {"src/b.go", 4}, {"README.md", 1}},
		Risk:      "HIGH",
		Decisions: []Decision{{ID: "D-1", Question: "Which region?", Answer: "b", By: "human"}},
		Linked:    []Linked{{Repo: "orders", PR: "https://github.com/acme/orders/pull/902"}},
		Rollback:  []string{"T-01: revert the commit"},
	})
	for _, want := range []string{
		"## Summary", "CHG-001", "spec/users.md", "## Why", "prd/v1.md",
		"## What changed", "- `src/` (2 files, 16 lines)", "- `README.md` (1 file, 1 line)",
		"## Blast radius", "HIGH", "## Decisions", "D-1 Which region? → b (human)",
		"## Tests and gates", "## Security notes", "## Side findings (not fixed here)",
		"## Linked PRs and merge order", "orders: https://github.com/acme/orders/pull/902",
		"## Rollback", "T-01: revert the commit",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("body lacks %q:\n%s", want, b)
		}
	}
	empty := Body(BodyInput{Change: "CHG-001", Title: "x", Repo: "users"})
	for _, want := range []string{"No decisions were asked or made.", "No other repo in this change.", "No rollback steps in the plan.", "unknown"} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty body lacks %q:\n%s", want, empty)
		}
	}
}
