// Package preflight is doc 04's git preflight, before Athel's first write in a repo
// (FR-GIT-002/003): others' active work on the repo, its overlap with the planned paths, the
// card when someone else's open work overlaps, and the report. Collecting the work (gh, git)
// is the caller's; everything here is pure, so it is tested without a network.
package preflight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// SchemaID is the report's format.
const SchemaID = "athel/preflight@1"

// Windows of doc 04's preflight step 3.
const (
	BranchWindow = 14 * 24 * time.Hour
	CommitWindow = 7 * 24 * time.Hour
)

// PR is an open pull request.
type PR struct {
	Number int      `yaml:"number" json:"number"`
	Author string   `yaml:"author" json:"author"`
	Head   string   `yaml:"head" json:"head"`
	Files  []string `yaml:"files" json:"files"`
}

// Branch is a branch someone pushed recently, with the files it changes against base.
type Branch struct {
	Name   string   `yaml:"name" json:"name"`
	Author string   `yaml:"author" json:"author"`
	Pushed string   `yaml:"pushed_at" json:"pushed_at"`
	Files  []string `yaml:"files" json:"files"`
}

// Commit is a recent commit to base.
type Commit struct {
	SHA    string   `yaml:"sha" json:"sha"`
	Author string   `yaml:"author" json:"author"`
	At     string   `yaml:"at" json:"at"`
	Files  []string `yaml:"files" json:"files"`
}

// Work is others' active work on one repo.
type Work struct {
	PRs      []PR
	Branches []Branch
	Commits  []Commit
}

// Overlap is active work touching planned paths. Open PRs and pushed branches are someone's
// unfinished work and block (doc 04 step 5); recent commits to base are already in the base
// Athel branches from, so they are reported for context only.
type Overlap struct {
	Kind     string   `yaml:"kind" json:"kind"` // pr | branch | commit
	Ref      string   `yaml:"ref" json:"ref"`
	Author   string   `yaml:"author" json:"author"`
	Files    []string `yaml:"files" json:"files"`
	Blocking bool     `yaml:"blocking" json:"blocking"`
}

// ParsePRs reads `gh pr list --json number,author,headRefName,files`. gh gives the author as
// {login} and files as [{path}]; plain strings are accepted too.
func ParsePRs(data []byte) ([]PR, error) {
	var raw []struct {
		Number int               `json:"number"`
		Author json.RawMessage   `json:"author"`
		Head   string            `json:"headRefName"`
		Files  []json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, unreadable(err)
	}
	out := make([]PR, 0, len(raw))
	for _, r := range raw {
		author, err := loginOrString(r.Author, "login")
		if err != nil {
			return nil, unreadable(err)
		}
		pr := PR{Number: r.Number, Author: author, Head: r.Head, Files: []string{}}
		for _, f := range r.Files {
			path, err := loginOrString(f, "path")
			if err != nil {
				return nil, unreadable(err)
			}
			pr.Files = append(pr.Files, path)
		}
		out = append(out, pr)
	}
	return out, nil
}

// loginOrString reads a JSON string, or an object's field key; null is "".
func loginOrString(raw json.RawMessage, key string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	v, _ := m[key].(string)
	return v, nil
}

func unreadable(err error) error {
	return aerr.Wrap(aerr.ToolUnreachable, "read the open pull requests from gh", err).
		WithNextAction("check `gh auth status` and that gh can list this repo's pull requests")
}

// ParseBranches reads `git for-each-ref --format=%(refname:strip=2)%09%(committerdate:iso-strict)%09%(authoremail)`
// and keeps the branches pushed within BranchWindow of now, except those in skip (base, and
// Athel's own branch). Files are filled in by the caller.
func ParseBranches(out string, now time.Time, skip []string) []Branch {
	var got []Branch
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 || slices.Contains(skip, f[0]) {
			continue
		}
		at, err := time.Parse(time.RFC3339, f[1])
		if err != nil || now.Sub(at) > BranchWindow {
			continue
		}
		got = append(got, Branch{Name: f[0], Author: strings.Trim(f[2], "<>"), Pushed: at.UTC().Format(time.RFC3339), Files: []string{}})
	}
	return got
}

// ParseCommits reads `git log --format=%x00%H%x09%ae%x09%cI --name-only`.
func ParseCommits(out string) []Commit {
	var got []Commit
	for _, rec := range strings.Split(out, "\x00") {
		header, files, _ := strings.Cut(rec, "\n")
		f := strings.Split(header, "\t")
		if len(f) != 3 {
			continue
		}
		at, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			continue
		}
		c := Commit{SHA: f[0], Author: f[1], At: at.UTC().Format(time.RFC3339), Files: []string{}}
		for _, name := range strings.Split(files, "\n") {
			if name = strings.TrimSpace(name); name != "" {
				c.Files = append(c.Files, name)
			}
		}
		got = append(got, c)
	}
	return got
}

// Overlaps lists the work that touches a path matching one of globs, in the order PRs,
// branches, commits.
func Overlaps(w Work, globs []string) []Overlap {
	var out []Overlap
	add := func(kind, ref, author string, files []string, blocking bool) {
		var hit []string
		for _, f := range files {
			if slices.ContainsFunc(globs, func(g string) bool { return plan.MatchGlob(g, f) }) {
				hit = append(hit, f)
			}
		}
		if len(hit) > 0 {
			out = append(out, Overlap{Kind: kind, Ref: ref, Author: author, Files: hit, Blocking: blocking})
		}
	}
	for _, p := range w.PRs {
		add("pr", "#"+strconv.Itoa(p.Number), p.Author, p.Files, true)
	}
	for _, b := range w.Branches {
		add("branch", b.Name, b.Author, b.Files, true)
	}
	for _, c := range w.Commits {
		add("commit", c.SHA, c.Author, c.Files, false)
	}
	return out
}

// Blocking counts the overlaps that block.
func Blocking(overlaps []Overlap) int {
	n := 0
	for _, o := range overlaps {
		if o.Blocking {
			n++
		}
	}
	return n
}

// Question is the preflight card's question for repo, so a re-run finds the card it raised.
func Question(repo string) string {
	return "Someone else is working on files this change plans to edit in " + repo + ". How should Athel go on?"
}

// CardDraft is the A4 card of doc 04 step 5: wait, coordinate, or narrow the plan. Waiting is
// the safe default; coordinating is recommended, since both pieces of work can then go on.
func CardDraft(repo string, overlaps []Overlap) decisions.Draft {
	var lines []string
	for _, o := range overlaps {
		if o.Blocking {
			lines = append(lines, fmt.Sprintf("%s %s by %s: %s", o.Kind, o.Ref, o.Author, strings.Join(o.Files, ", ")))
		}
	}
	return decisions.Draft{
		Rule: "A4", Step: "build", Question: Question(repo),
		Context: "Git preflight found open work on planned paths in " + repo + ": " + strings.Join(lines, "; ") + ".",
		Options: []decisions.Option{
			{Key: "a", Label: "Wait for that work to merge or close", Consequence: "Athel writes nothing in " + repo + " until the preflight is run again and finds no overlap"},
			{Key: "b", Label: "Coordinate with its author", Consequence: "Athel drafts a message to the author for you to send; building starts once you confirm the plan still holds"},
			{Key: "c", Label: "Narrow the plan", Consequence: "the plan changes to leave those files alone, and it needs your approval again"},
		},
		Recommendation: decisions.Recommendation{Option: "b", Reason: "both pieces of work can go on without conflicting edits"},
		Default:        "a", DefaultSafe: true, Blocks: []string{"build"},
	}
}

// Report is preflight/<repo>.md.
type Report struct {
	Change, Repo, Remote, Base, BaseHead, Branch, At string
	PlannedPaths                                     []string
	Work                                             Work
	Overlaps                                         []Overlap
	Card                                             string // the card raised, or ""
}

type block struct {
	Schema        string    `yaml:"schema"`
	Change        string    `yaml:"change"`
	Repo          string    `yaml:"repo"`
	Remote        string    `yaml:"remote"`
	Base          string    `yaml:"base"`
	BaseHead      string    `yaml:"base_head"`
	Branch        string    `yaml:"branch"`
	At            string    `yaml:"at"`
	PlannedPaths  []string  `yaml:"planned_paths"`
	OpenPRs       []PR      `yaml:"open_prs"`
	Branches      []Branch  `yaml:"branches"`
	RecentCommits []Commit  `yaml:"recent_commits"`
	Overlaps      []Overlap `yaml:"overlaps"`
	Clean         bool      `yaml:"clean"`
	Card          *string   `yaml:"card"`
}

// Render is the report as Markdown with its preflight@1 block, validated.
func (r Report) Render(reg *schema.Registry) ([]byte, error) {
	b := block{Schema: SchemaID, Change: r.Change, Repo: r.Repo, Remote: r.Remote, Base: r.Base, BaseHead: r.BaseHead,
		Branch: r.Branch, At: r.At, PlannedPaths: r.PlannedPaths, OpenPRs: orEmpty(r.Work.PRs), Branches: orEmpty(r.Work.Branches),
		RecentCommits: orEmpty(r.Work.Commits), Overlaps: orEmpty(r.Overlaps), Clean: Blocking(r.Overlaps) == 0}
	if r.Card != "" {
		b.Card = &r.Card
	}
	data, _ := yaml.Marshal(b) // plain strings, numbers and lists always encode
	if err := reg.ValidateYAML(SchemaID, data); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "preflight report for "+r.Repo, err)
	}
	var md bytes.Buffer
	fmt.Fprintf(&md, "# Preflight: %s (%s)\n\nBranch `%s` from `%s` at `%s`.\n\n", r.Repo, r.Change, r.Branch, r.Base, r.BaseHead)
	if n := Blocking(r.Overlaps); n > 0 {
		fmt.Fprintf(&md, "**Blocked:** %d overlap with someone else's open work. Card %s asks how to go on.\n\n", n, r.Card)
	} else {
		md.WriteString("**Clear:** no one else's open work touches the planned paths.\n\n")
	}
	for _, o := range r.Overlaps {
		fmt.Fprintf(&md, "- %s %s by %s: %s\n", o.Kind, o.Ref, o.Author, strings.Join(o.Files, ", "))
	}
	md.WriteString("\n## Report\n\n```yaml\n")
	md.Write(data)
	md.WriteString("```\n")
	return md.Bytes(), nil
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Check reads a stored report back: its preflight@1 block must validate and name change and
// repo, so a hand-made or stale file never stands in for a preflight.
func Check(reg *schema.Registry, change, repo string, md []byte) error {
	_, rest, ok := strings.Cut(string(md), "```yaml\n")
	data, _, closed := strings.Cut(rest, "```")
	if !ok || !closed {
		return aerr.New(aerr.StateSchema, "preflight/"+repo+".md has no report block").WithNextAction("run athel git preflight " + change + " " + repo)
	}
	if err := reg.ValidateYAML(SchemaID, []byte(data)); err != nil {
		return aerr.Wrap(aerr.StateSchema, "preflight/"+repo+".md", err).WithNextAction("run athel git preflight " + change + " " + repo)
	}
	var b block
	if err := yaml.Unmarshal([]byte(data), &b); err != nil || b.Change != change || b.Repo != repo {
		return aerr.New(aerr.StateSchema, "preflight/"+repo+".md is not the report of "+repo+" in "+change).
			WithNextAction("run athel git preflight " + change + " " + repo)
	}
	return nil
}
