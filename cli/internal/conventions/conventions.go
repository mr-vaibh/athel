// Package conventions is brownfield onboarding (doc 05 §Brownfield onboarding; FR-SPEC-006,
// F-21). Survey collects, from Athel's own cache clone, what the architect needs to describe a
// repo's conventions: build, lint and CI files (with content), the folder structure, recently
// changed files and test files. The architect writes conventions.md and map.md; the CLI stores
// them with the git blob of every surveyed config file, so they go stale the moment a build or
// lint config changes, or after 30 days.
package conventions

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Git runs git in a repo; injected so tests and the CLI choose how.
type Git func(dir string, args ...string) (int, string, error)

// MaxContent caps each config file's content in a survey.
const MaxContent = 8 * 1024

// StaleAfter is doc 05's refresh window for conventions.
const StaleAfter = 30 * 24 * time.Hour

// Input is one surveyed config file.
type Input struct {
	Path      string `json:"path" yaml:"path"`
	Kind      string `json:"kind,omitempty" yaml:"-"`
	Blob      string `json:"blob" yaml:"blob"`
	Content   string `json:"content,omitempty" yaml:"-"`
	Truncated bool   `json:"truncated,omitempty" yaml:"-"`
}

// Survey is what the architect reads before writing conventions.md and map.md.
type Survey struct {
	Repo    string   `json:"repo"`
	Commit  string   `json:"commit"`
	Inputs  []Input  `json:"inputs"`
	Folders []string `json:"folders"`
	Recent  []string `json:"recent"`
	Tests   []string `json:"tests"`
}

var (
	buildFiles = []string{"go.mod", "package.json", "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
		"Cargo.toml", "pyproject.toml", "setup.py", "requirements.txt", "Gemfile", "Makefile", "Dockerfile", "composer.json", "mix.exs"}
	lintFiles = []string{".golangci.yml", ".golangci.yaml", ".editorconfig", ".prettierrc", ".prettierrc.json", ".prettierrc.yml",
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", "eslint.config.js", "eslint.config.mjs",
		"biome.json", "ruff.toml", ".flake8", "setup.cfg", "tox.ini", ".rubocop.yml", "tsconfig.json", ".clang-format", "rustfmt.toml"}
	skipDirs = []string{"vendor", "node_modules", "dist", "build", "target", ".git", "__pycache__", ".venv", "third_party"}
)

// kindOf classifies a file as build, lint or ci config, or "".
func kindOf(p string) string {
	base := path.Base(p)
	switch {
	case strings.HasPrefix(p, ".github/workflows/") && (strings.HasSuffix(p, ".yml") || strings.HasSuffix(p, ".yaml")),
		p == ".gitlab-ci.yml", p == "Jenkinsfile", p == ".circleci/config.yml":
		return "ci"
	case slices.Contains(lintFiles, base) || strings.HasPrefix(base, "tsconfig") && strings.HasSuffix(base, ".json"):
		return "lint"
	case slices.Contains(buildFiles, base) || strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"):
		return "build"
	}
	return ""
}

// isTest reports a test file by the common naming of Go, JS/TS, Python, Java and Ruby.
func isTest(p string) bool {
	base := path.Base(p)
	for _, s := range []string{"_test.go", ".test.ts", ".test.tsx", ".test.js", ".spec.ts", ".spec.tsx", ".spec.js", "_test.py", "Test.java", "_spec.rb"} {
		if strings.HasSuffix(base, s) {
			return true
		}
	}
	return strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")
}

func skipped(p string) bool {
	for _, part := range strings.Split(p, "/") {
		if slices.Contains(skipDirs, part) {
			return true
		}
	}
	return false
}

// Take surveys the repo at dir (a bare cache clone) at HEAD.
func Take(git Git, repo, dir string) (Survey, error) {
	s := Survey{Repo: repo, Inputs: []Input{}, Folders: []string{}, Recent: []string{}, Tests: []string{}}
	commit, err := run(git, dir, "rev-parse", "HEAD")
	if err != nil {
		return s, err
	}
	s.Commit = strings.TrimSpace(commit)
	list, err := run(git, dir, "ls-tree", "-r", "--name-only", "HEAD")
	if err != nil {
		return s, err
	}
	files := strings.Fields(list)
	folders := map[string]bool{}
	for _, f := range files {
		if skipped(f) {
			continue
		}
		dirs := strings.Split(path.Dir(f), "/")
		for i := 1; i <= len(dirs) && i <= 3; i++ {
			if d := strings.Join(dirs[:i], "/"); d != "." {
				folders[d] = true
			}
		}
		if isTest(f) && len(s.Tests) < 200 {
			s.Tests = append(s.Tests, f)
		}
		if k := kindOf(f); k != "" {
			in, err := input(git, dir, f, k)
			if err != nil {
				return s, err
			}
			s.Inputs = append(s.Inputs, in)
		}
	}
	for d := range folders {
		s.Folders = append(s.Folders, d)
	}
	slices.Sort(s.Folders)
	s.Recent, err = recent(git, dir, files)
	return s, err
}

func input(git Git, dir, p, kind string) (Input, error) {
	blob, err := run(git, dir, "rev-parse", "HEAD:"+p)
	if err != nil {
		return Input{}, err
	}
	content, err := run(git, dir, "show", "HEAD:"+p)
	if err != nil {
		return Input{}, err
	}
	in := Input{Path: p, Kind: kind, Blob: strings.TrimSpace(blob), Content: content}
	if len(in.Content) > MaxContent {
		in.Content, in.Truncated = in.Content[:MaxContent], true
	}
	return in, nil
}

// recent lists the 20 most recently changed files that still exist.
func recent(git Git, dir string, files []string) ([]string, error) {
	log, err := run(git, dir, "log", "-n", "200", "--name-only", "--pretty=format:")
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, f := range strings.Fields(log) {
		if len(out) == 20 {
			break
		}
		if slices.Contains(files, f) && !slices.Contains(out, f) && !skipped(f) {
			out = append(out, f)
		}
	}
	return out, nil
}

func run(git Git, dir string, args ...string) (string, error) {
	code, out, err := git(dir, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", &GitError{Args: args, Code: code, Out: out}
	}
	return out, nil
}

// GitError is a git command that exited non-zero.
type GitError struct {
	Args []string
	Code int
	Out  string
}

func (e *GitError) Error() string {
	return "git " + strings.Join(e.Args, " ") + " exited " + strconv.Itoa(e.Code) + ": " + strings.TrimSpace(e.Out)
}

// Meta is .athel/repos/<repo>/conventions.yaml (athel/conventions@1): what the conventions were
// written from, so the CLI can tell when they are stale.
type Meta struct {
	Schema      string  `yaml:"schema" json:"schema"`
	Repo        string  `yaml:"repo" json:"repo"`
	NewProject  bool    `yaml:"new_project" json:"new_project"`
	Commit      string  `yaml:"commit" json:"commit"`
	SurveyedAt  string  `yaml:"surveyed_at" json:"surveyed_at"`
	Inputs      []Input `yaml:"inputs" json:"inputs"`
	Conventions string  `yaml:"conventions_sha256" json:"conventions_sha256"`
	Map         string  `yaml:"map_sha256" json:"map_sha256"`
}

// Stale says why conventions need refreshing, or "" when they are fresh: older than 30 days, or
// a surveyed config file changed or disappeared at blobAt's HEAD. A new project's conventions
// only age.
func Stale(m Meta, now, surveyed time.Time, blobAt func(path string) (string, bool)) string {
	if now.Sub(surveyed) > StaleAfter {
		return "written more than 30 days ago"
	}
	for _, in := range m.Inputs {
		blob, ok := blobAt(in.Path)
		switch {
		case !ok:
			return in.Path + " no longer exists"
		case blob != in.Blob:
			return in.Path + " changed since the conventions were written"
		}
	}
	return ""
}

// Remote reads a repo as a spec or command names it: a git URL (normalised by norm) or the
// host/org/repo form Athel prints. Anything else is kept as given, for a new project's name.
func Remote(name string, norm func(string) string) string {
	if r := norm(name); r != "" {
		return r
	}
	if r := norm("https://" + name); r != "" && !strings.Contains(name, "://") {
		return r
	}
	return name
}

// Dir is the folder name for a repo under .athel/repos/: its normalised remote with / as -.
func Dir(remote string) string { return strings.ReplaceAll(remote, "/", "-") }
