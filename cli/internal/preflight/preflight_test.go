package preflight

import (
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func TestParsePRs(t *testing.T) {
	// The real gh nests the author and the files; the shop fixture's fake flattens them.
	real := `[{"number":812,"author":{"login":"teammate"},"headRefName":"teammate/profile-phone","files":[{"path":"api/user.go","additions":1}],"url":"https://github.com/acme/users/pull/812"}]`
	flat := `[{"number":812,"author":"teammate","headRefName":"teammate/profile-phone","files":["api/user.go"]}]`
	for name, in := range map[string]string{"real": real, "flat": flat} {
		prs, err := ParsePRs([]byte(in))
		if err != nil || len(prs) != 1 || prs[0].Number != 812 || prs[0].Author != "teammate" ||
			prs[0].Head != "teammate/profile-phone" || strings.Join(prs[0].Files, ",") != "api/user.go" {
			t.Errorf("%s: %+v %v", name, prs, err)
		}
	}
	for _, bad := range []string{"{", `[{"author":3}]`, `[{"files":[3]}]`} {
		if _, err := ParsePRs([]byte(bad)); !aerr.HasCode(err, aerr.ToolUnreachable) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if prs, err := ParsePRs([]byte("[]")); err != nil || len(prs) != 0 {
		t.Errorf("no PRs: %v %v", prs, err)
	}
}

func TestParseBranches(t *testing.T) {
	out := "main\t2026-11-13T09:00:00Z\t<dev@x>\n" +
		"teammate/profile-phone\t2026-11-10T15:00:00+00:00\t<mate@x>\n" +
		"old/work\t2026-10-01T15:00:00Z\t<old@x>\n" +
		"athel/CHG-001-x\t2026-11-14T10:00:00Z\t<athel@x>\n" +
		"broken\tnot-a-date\t<x@x>\n\n"
	got := ParseBranches(out, now, []string{"main", "athel/CHG-001-x"})
	if len(got) != 1 || got[0].Name != "teammate/profile-phone" || got[0].Author != "mate@x" || got[0].Pushed != "2026-11-10T15:00:00Z" {
		t.Errorf("only others' branches pushed in the last 14 days: %+v", got)
	}
}

func TestParseCommits(t *testing.T) {
	out := "\x00aaa\tdev@x\t2026-11-12T08:00:00Z\n\napi/user.go\nREADME.md\n\x00bbb\tops@x\t2026-11-11T08:00:00+01:00\n\ndeploy/x.yaml\n"
	got := ParseCommits(out)
	if len(got) != 2 || got[0].SHA != "aaa" || strings.Join(got[0].Files, ",") != "api/user.go,README.md" || got[1].At != "2026-11-11T07:00:00Z" {
		t.Errorf("%+v", got)
	}
	if got := ParseCommits("\x00short\n\x00ccc\tx@x\tyesterday\n"); len(got) != 0 {
		t.Errorf("a malformed header is skipped: %+v", got)
	}
}

func TestOverlaps(t *testing.T) {
	w := Work{
		PRs:      []PR{{Number: 812, Author: "teammate", Head: "teammate/profile-phone", Files: []string{"api/user.go", "README.md"}}, {Number: 9, Author: "x", Files: []string{"docs/a.md"}}},
		Branches: []Branch{{Name: "spike/phone", Author: "a@x", Files: []string{"api/phone.go"}}},
		Commits:  []Commit{{SHA: "aaa", Author: "dev@x", Files: []string{"api/user.go"}}, {SHA: "bbb", Files: []string{"deploy/x"}}},
	}
	got := Overlaps(w, []string{"api/**"})
	want := []string{"pr #812 blocking api/user.go", "branch spike/phone blocking api/phone.go", "commit aaa recent api/user.go"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, o := range got {
		kind := "recent"
		if o.Blocking {
			kind = "blocking"
		}
		if s := o.Kind + " " + o.Ref + " " + kind + " " + strings.Join(o.Files, ","); s != want[i] {
			t.Errorf("overlap %d = %q, want %q", i, s, want[i])
		}
	}
	if Blocking(got) != 2 {
		t.Errorf("blocking = %d", Blocking(got))
	}
}

func TestReportRendersAndValidates(t *testing.T) {
	r := Report{Change: "CHG-001", Repo: "users", Remote: "github.com/acme/users", Base: "main", BaseHead: strings.Repeat("a", 40),
		Branch: "athel/CHG-001-x", At: "2026-11-14T12:00:00.000Z", PlannedPaths: []string{"api/**"},
		Work:     Work{PRs: []PR{{Number: 812, Author: "teammate", Head: "h", Files: []string{"api/user.go"}}}},
		Overlaps: []Overlap{{Kind: "pr", Ref: "#812", Author: "teammate", Files: []string{"api/user.go"}, Blocking: true}}, Card: "D-3"}
	md, err := r.Render(schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Preflight: users (CHG-001)", "**Blocked:** 1 overlap with someone else's open work", "D-3", "schema: athel/preflight@1", "#812"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("report misses %q:\n%s", want, md)
		}
	}
	r.Overlaps, r.Card = nil, ""
	md, err = r.Render(schema.NewRegistry())
	if err != nil || !strings.Contains(string(md), "**Clear:** no one else's open work touches the planned paths") || !strings.Contains(string(md), "card: null") {
		t.Errorf("clean report: %v\n%s", err, md)
	}
	r.BaseHead = "not-a-sha"
	if _, err := r.Render(schema.NewRegistry()); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("an invalid report is refused: %v", err)
	}
}

func TestCardDraft(t *testing.T) {
	d := CardDraft("users", []Overlap{{Kind: "pr", Ref: "#812", Author: "teammate", Files: []string{"api/user.go"}, Blocking: true},
		{Kind: "commit", Ref: "aaa", Files: []string{"api/x.go"}}})
	if d.Rule != "A4" || d.Step != "build" || len(d.Options) != 3 || d.Recommendation.Option != "b" || d.Default != "a" || !d.DefaultSafe ||
		strings.Join(d.Blocks, ",") != "build" || !strings.Contains(d.Context, "#812 by teammate: api/user.go") || strings.Contains(d.Context, "aaa") {
		t.Errorf("%+v", d)
	}
	if d.Question != Question("users") {
		t.Errorf("question %q", d.Question)
	}
}

func TestCheckReadsTheReportBack(t *testing.T) {
	reg := schema.NewRegistry()
	r := Report{Change: "CHG-001", Repo: "users", Remote: "github.com/acme/users", Base: "main", BaseHead: strings.Repeat("a", 40),
		Branch: "athel/CHG-001-x", At: "2026-11-14T12:00:00.000Z", PlannedPaths: []string{"api/**"}}
	md, err := r.Render(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(reg, "CHG-001", "users", md); err != nil {
		t.Errorf("a rendered report checks: %v", err)
	}
	for name, bad := range map[string][]byte{
		"no block":      []byte("# Preflight\n"),
		"invalid block": []byte("```yaml\nschema: athel/preflight@1\n```\n"),
		"other repo":    md,
	} {
		repo := "users"
		if name == "other repo" {
			repo = "orders"
		}
		if err := Check(reg, "CHG-001", repo, bad); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
