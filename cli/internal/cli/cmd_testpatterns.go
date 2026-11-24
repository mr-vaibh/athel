package cli

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/conventions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/gates"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// A repo's own test locations (M5-plan G15; ADR 024 follow-up). They widen where QA may write,
// so the CLI accepts a pattern only if it names a test location and matches a real file of the
// repo: a pattern such as "src/**" can never make product code a "test".

const testPatternsSchema = "athel/test-patterns@1"

type testPatterns struct {
	Schema   string   `yaml:"schema" json:"schema"`
	Repo     string   `yaml:"repo" json:"repo"`
	Patterns []string `yaml:"patterns" json:"patterns"`
}

// testFolders are folder names that hold tests; testName is a file name made for tests: a test
// word set off by . _ or -, or a capitalised Test, Tests, Spec or IT before the extension.
var (
	testFolders = []string{"test", "tests", "spec", "specs", "__tests__", "e2e", "it", "integration", "testing"}
	testName    = regexp.MustCompile(`((^|[._-])(?i:test|tests|spec|specs)([._-]|$))|(Test|Tests|Spec|IT)\.[A-Za-z]+$`)
)

// namesTests reports whether a pattern names a test location in one of its parts.
func namesTests(p string) bool {
	for _, part := range strings.Split(p, "/") {
		bare := strings.ReplaceAll(part, "*", "")
		if slices.Contains(testFolders, strings.ToLower(bare)) || testName.MatchString(bare) {
			return true
		}
	}
	return false
}

func cmdConventionsTests(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("conventions tests"), args, -1)
	if err != nil {
		return nil, err
	}
	if len(pos) < 2 {
		return nil, usagef("conventions tests: a repo and at least one pattern")
	}
	remote := r.resolveRepo(pos[0])
	files, err := r.repoFiles(remote)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, p := range pos[1:] {
		if why := badTestPattern(p, files); why != "" {
			problems = append(problems, p+": "+why)
		}
	}
	if len(problems) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "test patterns refused: "+strings.Join(problems, "; ")).
			WithNextAction("name the repo's own test folders or file names, each matching at least one test file")
	}
	doc := testPatterns{Schema: testPatternsSchema, Repo: remote, Patterns: pos[1:]}
	b, err := yaml.Marshal(doc)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "encode tests.yaml", err)
	}
	if err := r.registry().ValidateYAML(testPatternsSchema, b); err != nil {
		return nil, err
	}
	if _, err := r.writeRepoFiles(remote, map[string][]byte{"tests.yaml": b}); err != nil {
		return nil, err
	}
	return doc, nil
}

// badTestPattern says why p cannot be a test location, or "".
func badTestPattern(p string, files []string) string {
	switch {
	case strings.HasPrefix(p, "/") || slices.Contains(strings.Split(p, "/"), ".."):
		return "a pattern is relative to the repo root and stays inside it"
	case !namesTests(p):
		return "it names no test folder or test file name (test, tests, spec, __tests__, e2e, it, integration)"
	case !slices.ContainsFunc(files, func(f string) bool { return plan.MatchGlob(p, f) }):
		return "it matches no file of the repo on its base branch"
	}
	return ""
}

// repoFiles lists the files of a repo's base branch in Athel's cache clone.
func (r *runCtx) repoFiles(remote string) ([]string, error) {
	pol, err := r.policySet()
	if err != nil {
		return nil, err
	}
	cache, err := r.cachePath(remote)
	if err != nil {
		return nil, err
	}
	code, out, err := gitro.Run(cache, "ls-tree", "-r", "--name-only", "refs/heads/"+pol.Git.BaseBranch)
	if err != nil || code != 0 {
		return nil, aerr.New(aerr.StateNotFound, "no cache clone of "+remote+" to check the patterns against").
			WithNextAction("athel repos cache " + remote)
	}
	return strings.Fields(out), nil
}

// testMatcher is the repo's test rule: the language defaults plus its saved patterns.
func (r *runCtx) testMatcher(repo string) func(string) bool {
	remote := r.resolveRepo(repo)
	ws, err := r.workspaceDir()
	if err != nil {
		return gates.IsTestFile
	}
	b, err := os.ReadFile(filepath.Join(ws.Dir(), "repos", conventions.Dir(remote), "tests.yaml"))
	if errors.Is(err, os.ErrNotExist) || err != nil {
		return gates.IsTestFile
	}
	var doc testPatterns
	if r.registry().ValidateYAML(testPatternsSchema, b) != nil || yaml.Unmarshal(b, &doc) != nil {
		return gates.IsTestFile
	}
	return gates.TestMatcher(doc.Patterns)
}
