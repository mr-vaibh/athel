package defects

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

// knownRules are the rule IDs of the reviewer's table, which TestRuleTablesMatchCLI keeps equal
// to the CLI's list.
func knownRules(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../../../../plugin/agents/reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range regexp.MustCompile("(?m)^\\| `([a-z]+\\.[a-z-]+)` \\|").FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	return out
}

// hints would tell the reviewer where to look; the corpus must read like ordinary code.
var hints = regexp.MustCompile(`(?i)\b(bug|defect|vulnerab\w*|insecure|unsafe|hack|fixme|xxx|planted|injection|seeded|on purpose|intentional\w*)\b`)

func TestCorpusShape(t *testing.T) {
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	rules := knownRules(t)
	defects, clean := 0, 0
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] {
			t.Errorf("%s twice", c.ID)
		}
		seen[c.ID] = true
		checkCase(t, c)
		switch c.Kind {
		case "defect":
			defects++
			checkDefect(t, c, rules)
		case "clean":
			clean++
			if c.Planted != nil || len(c.Rules) > 0 || !strings.HasPrefix(c.ID, "C") {
				t.Errorf("%s: a clean case has a C ID, no planted lines and no rules", c.ID)
			}
		default:
			t.Errorf("%s: kind is defect or clean", c.ID)
		}
	}
	if defects < 24 || clean < 6 {
		t.Errorf("%d defects and %d clean diffs, want ≥ 24 and ≥ 6 (M4-plan EC-2)", defects, clean)
	}
}

// checkCase is what every case needs: a repo, files, a requirement, no hint, and nothing that
// overlaps the teammate's open PR.
func checkCase(t *testing.T, c Case) {
	t.Helper()
	switch {
	case c.Repo != "users" && c.Repo != "orders":
		t.Errorf("%s: repo is users or orders", c.ID)
	case len(c.Files) == 0 || c.Requirement == "" || c.Criterion.Then == "":
		t.Errorf("%s: files, a requirement and its criterion", c.ID)
	case slices.Contains(c.FileNames(), "api/user.go"):
		t.Errorf("%s: api/user.go overlaps the teammate's open PR 812", c.ID)
	}
	for name, body := range c.Files {
		if m := hints.FindString(body); m != "" {
			t.Errorf("%s: %s says %q", c.ID, name, m)
		}
	}
}

func checkDefect(t *testing.T, c Case, rules []string) {
	t.Helper()
	switch {
	case !strings.HasPrefix(c.ID, "D") || c.Category == "" || len(c.Rules) == 0 || c.Planted == nil:
		t.Errorf("%s: a defect has a D ID, a category, rules and planted lines", c.ID)
		return
	case c.Files[c.Planted.File] == "":
		t.Errorf("%s: planted file %s is not one of its files", c.ID, c.Planted.File)
		return
	case !testsAllowed(c):
		t.Errorf("%s: tests %s is one of its test files, for a defect with a tests rule", c.ID, c.Planted.Tests)
	}
	n := len(strings.Split(strings.TrimRight(c.Files[c.Planted.File], "\n"), "\n"))
	if a, b := c.Planted.Lines[0], c.Planted.Lines[1]; a < 1 || b < a || b > n || b-a > 12 {
		t.Errorf("%s: planted lines %d-%d in a %d-line file (at most 13 lines)", c.ID, a, b, n)
	}
	for _, r := range c.Rules {
		if !slices.Contains(rules, r) {
			t.Errorf("%s: %s is not a rule ID", c.ID, r)
		}
	}
}

// testsAllowed: a finding on the test file counts only for a missing test, in one of the case's
// own test files.
func testsAllowed(c Case) bool {
	return c.Planted.Tests == "" || c.Files[c.Planted.Tests] != "" && strings.HasSuffix(c.Planted.Tests, "_test.go") &&
		slices.ContainsFunc(c.Rules, func(r string) bool { return strings.HasPrefix(r, "tests.") })
}

// TestCorpusBuilds applies each case to a copy of its fixture repo: it formats, builds, vets and
// passes its own tests, so every defect is one a compiler and the tests miss.
func TestCorpusBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("builds every case")
	}
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), c.Repo)
			if out, err := exec.Command("cp", "-R", filepath.Join(shop.Sources(), c.Repo), dir).CombinedOutput(); err != nil {
				t.Fatalf("%v %s", err, out)
			}
			for name, body := range c.Files {
				p := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, argv := range [][]string{{"gofmt", "-l", "."}, {"go", "vet", "./..."}, {"go", "test", "-count=1", "./..."}} {
				cmd := exec.Command(argv[0], argv[1:]...)
				cmd.Dir, cmd.Env = dir, append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
				out, err := cmd.CombinedOutput()
				if err != nil || (argv[0] == "gofmt" && len(strings.TrimSpace(string(out))) > 0) {
					t.Errorf("%v: %v\n%s", argv, err, out)
				}
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	p := Planted{File: "addresses/store.go", Lines: [2]int{10, 14}}
	cases := map[string]bool{
		"addresses/store.go:12": true, "addresses/store.go:8-10": true, "addresses/store.go:14-30": true,
		"addresses/store.go:15": false, "addresses/store.go:1-9": false, "addresses/other.go:12": false,
		"addresses/store.go": false, "addresses/store.go:x": false, "addresses/store.go:3-y": false,
	}
	for loc, want := range cases {
		if got := p.Overlaps(loc); got != want {
			t.Errorf("%s: %v", loc, got)
		}
	}
	// A missing test may be cited where it belongs: anywhere in the case's test file.
	p.Tests = "addresses/store_test.go"
	for loc, want := range map[string]bool{
		"addresses/store_test.go:3": true, "addresses/store_test.go:40-52": true, "addresses/store_test.go": false,
		"addresses/store.go:12": true, "addresses/other_test.go:3": false,
	} {
		if got := p.Overlaps(loc); got != want {
			t.Errorf("with tests %s: %v", loc, got)
		}
	}
}

// TestDefectsStayOutOfThePlugin (M4-plan §10): no planted line appears in what the reviewer and
// security agents read (agents, skills, the CLI's templates), so an example never hands them
// the answer.
func TestDefectsStayOutOfThePlugin(t *testing.T) {
	cases, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var corpus strings.Builder
	for _, root := range []string{"../../../../plugin/agents", "../../../../plugin/skills", "../../../../cli/internal"} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || (!strings.HasSuffix(p, ".md") && !strings.HasSuffix(p, ".yaml")) {
				return err
			}
			b, err := os.ReadFile(p)
			corpus.Write(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	text := corpus.String()
	for _, c := range cases {
		if c.Planted == nil {
			continue
		}
		lines := strings.Split(c.Files[c.Planted.File], "\n")
		for i := c.Planted.Lines[0] - 1; i < c.Planted.Lines[1] && i < len(lines); i++ {
			if l := strings.TrimSpace(lines[i]); len(l) >= 25 && strings.Contains(text, l) {
				t.Errorf("%s: the plugin contains its planted line %q", c.ID, l)
			}
		}
	}
}
