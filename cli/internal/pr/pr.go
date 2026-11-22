// Package pr builds Athel's pull requests (doc 04 §Git policy): the size count against the
// per-PR limit without generated files and lockfiles (FR-GIT-009), the split proposal when a
// diff is over it, the title, and the body with doc 04's sections. It holds no git or GitHub
// calls; the CLI collects the inputs.
package pr

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// File is one changed file and its changed lines.
type File struct {
	Path  string
	Lines int
}

// Counted is a diff's size: the files and lines that count, and the excluded files.
type Counted struct {
	Lines    int
	Files    []File
	Excluded []string
}

// Size reads `git diff --numstat` and counts changed lines (added plus deleted), leaving out
// files that match an exclusion: a pattern with a "/" matches the path, any other the file
// name. Binary files count as 0 lines.
func Size(numstat string, exclude []string) Counted {
	var c Counted
	for _, line := range strings.Split(numstat, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		if excluded(f[2], exclude) {
			c.Excluded = append(c.Excluded, f[2])
			continue
		}
		added, _ := strconv.Atoi(f[0]) // "-" for a binary file: 0
		deleted, _ := strconv.Atoi(f[1])
		c.Files = append(c.Files, File{Path: f[2], Lines: added + deleted})
		c.Lines += added + deleted
	}
	return c
}

func excluded(p string, patterns []string) bool {
	return slices.ContainsFunc(patterns, func(g string) bool {
		if strings.Contains(g, "/") {
			return plan.MatchGlob(g, p)
		}
		ok, _ := path.Match(g, path.Base(p))
		return ok
	})
}

// TaskLines is the size of one plan task's commits.
type TaskLines struct {
	Task  string
	Lines int
}

// Group is one proposed PR: tasks in plan order, their lines, and whether one task alone is
// over the limit (it needs splitting in the plan, or the limit raised at approval).
type Group struct {
	Tasks     []string `json:"tasks"`
	Lines     int      `json:"lines"`
	Oversized bool     `json:"oversized"`
}

// Split groups tasks, in order, into PRs of at most max lines each (FR-GIT-009: "a big PRD
// becomes several ordered PRs").
func Split(tasks []TaskLines, max int) []Group {
	var out []Group
	for _, t := range tasks {
		if n := len(out); n > 0 && !out[n-1].Oversized && out[n-1].Lines+t.Lines <= max {
			out[n-1].Tasks = append(out[n-1].Tasks, t.Task)
			out[n-1].Lines += t.Lines
			continue
		}
		out = append(out, Group{Tasks: []string{t.Task}, Lines: t.Lines, Oversized: t.Lines > max})
	}
	return out
}

// Title is the PR title in git.yaml's conventional format.
func Title(repo, changeTitle string) string {
	summary := strings.TrimSpace(changeTitle)
	if summary == "" {
		summary = "change"
	}
	return "feat(" + repo + "): " + strings.ToLower(summary[:1]) + summary[1:]
}

// Decision is one decision the body lists.
type Decision struct {
	ID, Question, Answer, By string
}

// Linked is another repo's PR of the same change.
type Linked struct {
	Repo, PR string
}

// BodyInput is what the body is made of.
type BodyInput struct {
	Change, Title, Repo, Spec, PRD, Risk string
	Files                                []File
	Decisions                            []Decision
	Linked                               []Linked
	Rollback                             []string
}

// Body is the PR description with doc 04's sections, in order.
func Body(in BodyInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Summary\n\n%s (%s) in %s. Spec: `%s`.\n\n", in.Title, in.Change, in.Repo, orNone(in.Spec, "none"))
	fmt.Fprintf(&b, "## Why\n\nThe PRD: `%s`.\n\n", orNone(in.PRD, "none"))
	b.WriteString("## What changed\n\n")
	for _, g := range groups(in.Files) {
		b.WriteString(g + "\n")
	}
	fmt.Fprintf(&b, "\n## Blast radius\n\nRisk level: %s. Details: `blast-radius.md` in the change folder.\n\n", orNone(in.Risk, "unknown"))
	b.WriteString("## Decisions\n\n")
	if len(in.Decisions) == 0 {
		b.WriteString("No decisions were asked or made.\n")
	}
	for _, d := range in.Decisions {
		fmt.Fprintf(&b, "- %s %s → %s (%s)\n", d.ID, d.Question, d.Answer, d.By)
	}
	b.WriteString("\n## Tests and gates\n\nNo gate results yet.\n\n## Security notes\n\nNo security report yet.\n\n")
	b.WriteString("## Side findings (not fixed here)\n\nNone recorded.\n\n## Linked PRs and merge order\n\n")
	if len(in.Linked) == 0 {
		b.WriteString("No other repo in this change.\n")
	}
	for i, l := range in.Linked {
		fmt.Fprintf(&b, "%d. %s: %s\n", i+1, l.Repo, l.PR)
	}
	b.WriteString("\n## Rollback\n\n")
	if len(in.Rollback) == 0 {
		b.WriteString("No rollback steps in the plan.\n")
	}
	for _, r := range in.Rollback {
		b.WriteString("- " + r + "\n")
	}
	return b.String()
}

// groups summarises files by top-level folder, sorted.
func groups(files []File) []string {
	type sum struct{ files, lines int }
	by := map[string]*sum{}
	for _, f := range files {
		key := f.Path
		if dir, _, ok := strings.Cut(f.Path, "/"); ok {
			key = dir + "/"
		}
		if by[key] == nil {
			by[key] = &sum{}
		}
		by[key].files++
		by[key].lines += f.Lines
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("- `%s` (%s, %s)", k, plural(by[k].files, "file"), plural(by[k].lines, "line")))
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func orNone(s, none string) string {
	if s == "" {
		return none
	}
	return s
}
