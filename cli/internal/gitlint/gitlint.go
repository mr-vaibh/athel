// Package gitlint checks Athel's commits, branch names and PR titles against git.yaml
// (FR-GIT-005, FR-BUILD-012), and what a commit adds: no file over 1 MB and nothing that looks
// like a secret (doc 04 edge case). Every check returns the problems in plain words; none is a
// model judgement.
package gitlint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/scrub"
)

// MaxFileBytes is doc 04's pre-commit limit: a file over 1 MB never goes into a commit.
const MaxFileBytes = 1 << 20

// conventionalTypes are the types git.yaml's conventional format names (doc 04).
var conventional = regexp.MustCompile(`^(feat|fix|refactor|test|docs|chore)(\([A-Za-z0-9._/-]+\))?!?: \S`)

const conventionalHelp = "use type(scope): summary, with type one of feat, fix, refactor, test, docs, chore"

// Rules are git.yaml's commit settings.
type Rules struct {
	Format   string   // conventional | free
	Trailers []string // templates such as "Athel-Change: {change_id}"
	MaxFiles int      // 0 means no limit
}

// Message checks a commit message: the subject (conventional format), and every trailer of
// the template in the message's last paragraph, where git reads trailers. {change_id} must be
// this change and {task_id} a task of the approved plan in this repo; any other value must
// match literally. Lines starting with # are git's comments and are ignored.
func Message(msg string, r Rules, change string, tasks []string) []string {
	var lines []string
	for _, l := range strings.Split(msg, "\n") {
		if !strings.HasPrefix(l, "#") {
			lines = append(lines, strings.TrimRight(l, " \t\r"))
		}
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return []string{"the commit message is empty"}
	}
	var problems []string
	subject, _, _ := strings.Cut(text, "\n")
	if r.Format == "conventional" && !conventional.MatchString(subject) {
		problems = append(problems, fmt.Sprintf("subject %q is not conventional: %s", subject, conventionalHelp))
	}
	trailers := trailersOf(text)
	for _, tmpl := range r.Trailers {
		key, want, _ := strings.Cut(tmpl, ": ")
		got, ok := trailers[key]
		switch {
		case !ok:
			problems = append(problems, "missing trailer "+key+" in the last paragraph of the message")
		case want == "{change_id}" && got != change:
			problems = append(problems, "trailer "+key+": "+got+" names another change; this is "+change)
		case want == "{task_id}" && !slices.Contains(tasks, got):
			problems = append(problems, "trailer "+key+": "+got+" is not a task of the approved plan in this repo ("+strings.Join(tasks, ", ")+")")
		case want != "{change_id}" && want != "{task_id}" && got != want:
			problems = append(problems, "trailer "+key+": "+got+" should be "+want)
		}
	}
	return problems
}

// trailersOf reads "Key: value" lines from the last paragraph, when the message has a body.
func trailersOf(text string) map[string]string {
	out := map[string]string{}
	paras := strings.Split(text, "\n\n")
	if len(paras) < 2 {
		return out
	}
	for _, l := range strings.Split(paras[len(paras)-1], "\n") {
		if k, v, ok := strings.Cut(l, ": "); ok && !strings.Contains(k, " ") {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// Title checks a PR title against the commit format.
func Title(title, format string) []string {
	switch {
	case strings.TrimSpace(title) == "":
		return []string{"the PR title is empty"}
	case format == "conventional" && !conventional.MatchString(title):
		return []string{fmt.Sprintf("PR title %q is not conventional: %s", title, conventionalHelp)}
	}
	return nil
}

// Branch checks a branch name against a git.yaml template for change: {change_id} is the
// change, {slug} lower-case words joined by hyphens, {finding_id} a side finding (SF-n).
func Branch(name, template, change string) []string {
	re := regexp.QuoteMeta(template)
	re = strings.NewReplacer(regexp.QuoteMeta("{change_id}"), regexp.QuoteMeta(change),
		regexp.QuoteMeta("{slug}"), `[a-z0-9]+(-[a-z0-9]+)*`,
		regexp.QuoteMeta("{finding_id}"), `SF-[1-9][0-9]*`).Replace(re)
	if !regexp.MustCompile("^" + re + "$").MatchString(name) {
		return []string{fmt.Sprintf("branch %q does not follow git.yaml's %q for %s", name, template, change)}
	}
	return nil
}

// Staged is one file a commit adds or changes: its path, its size and the lines it adds.
type Staged struct {
	Path  string
	Size  int64
	Added string
}

// StagedFiles checks what a commit adds: the number of files, no file over 1 MB, and no
// added line that looks like a secret (M1's scrubber patterns). Secrets are named by kind,
// never quoted.
func StagedFiles(files []Staged, r Rules) []string {
	var problems []string
	if r.MaxFiles > 0 && len(files) > r.MaxFiles {
		problems = append(problems, fmt.Sprintf("the commit has %d files; git.yaml allows at most %d per commit: split it into smaller steps", len(files), r.MaxFiles))
	}
	s := scrub.New()
	for _, f := range files {
		if f.Size > MaxFileBytes {
			problems = append(problems, fmt.Sprintf("%s is over 1 MB (%d bytes); large files never go into a commit", f.Path, f.Size))
		}
		if _, found := s.String(f.Added); len(found) > 0 {
			kinds := make([]string, 0, len(found))
			for _, k := range found {
				kinds = append(kinds, k.Kind)
			}
			problems = append(problems, f.Path+" adds what looks like a secret ("+strings.Join(kinds, ", ")+"); remove it and read it from the secret store at run time")
		}
	}
	return problems
}
