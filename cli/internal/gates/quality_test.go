package gates

import (
	"strconv"
	"strings"
	"testing"
)

const profile = `mode: set
example.com/users/src/a.go:3.14,5.2 1 1
example.com/users/src/a.go:7.14,9.2 1 0
example.com/users/src/a.go:11.20,13.2 2 1
example.com/users/api/b.go:1.1,2.2 1 0
`

func TestParseProfile(t *testing.T) {
	p, err := ParseProfile(profile, "example.com/users")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Total(); got != 60 {
		t.Errorf("3 of 5 statements: %v", got)
	}
	if len(p.Blocks["src/a.go"]) != 3 || len(p.Blocks["api/b.go"]) != 1 {
		t.Errorf("%+v", p.Blocks)
	}
	if _, err := ParseProfile("mode: set\nnot a block\n", "m"); err == nil {
		t.Error("a malformed profile must fail")
	}
	if got := (Profile{}).Total(); got != 100 {
		t.Errorf("no statements: %v", got)
	}
}

func TestChangedLines(t *testing.T) {
	diff := "diff --git a/src/a.go b/src/a.go\n+++ b/src/a.go\n@@ -3,0 +4,2 @@\n+x\n+y\n@@ -10 +12 @@\n-old\n+new\n" +
		"diff --git a/src/a_test.go b/src/a_test.go\n+++ b/src/a_test.go\n@@ -1 +1,3 @@\n" +
		"diff --git a/README.md b/README.md\n+++ b/README.md\n@@ -1 +1 @@\n" +
		"diff --git a/src/gone.go b/src/gone.go\n+++ /dev/null\n@@ -1,3 +0,0 @@\n"
	got := ChangedLines(diff)
	if strings.Join(intsToStrings(got["src/a.go"]), ",") != "4,5,12" || len(got) != 1 {
		t.Errorf("changed lines of Go files that are not tests: %v", got)
	}
}

func TestChangedCoverage(t *testing.T) {
	p, _ := ParseProfile(profile, "example.com/users")
	covered, uncovered := p.ChangedCoverage(map[string][]int{"src/a.go": {4, 8, 12, 20}})
	if covered != 2 || uncovered != 1 {
		t.Errorf("line 4 and 12 covered, 8 not, 20 is no statement: %d %d", covered, uncovered)
	}
}

const before = `package src

func Small() int { return 1 }

func Grows(a, b int) int {
	if a > 0 {
		return 1
	}
	return 2
}
`

const after = `package src

func Small() int { return 1 }

func Grows(a, b int) int {
	if a > 0 && b > 0 || a < -1 {
		return 1
	}
	for i := 0; i < a; i++ {
		switch {
		case i == 1, i == 2:
		case i == 3:
		case i == 4:
		case i == 5:
		case i == 6:
		case i == 7:
		case i == 8:
		case i == 9:
		default:
		}
	}
	if b > 1 {
		return 3
	}
	return 2
}

func (s *T) Method() {}
`

func TestComplexity(t *testing.T) {
	fs, err := Functions("a.go", after)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Function{}
	for _, f := range fs {
		byName[f.Name] = f
	}
	if byName["Small"].Complexity != 1 || byName["Grows"].Complexity != 14 || byName["T.Method"].Complexity != 1 {
		t.Errorf("%+v", fs)
	}
	odd := "package src\n\nfunc External()\n\ntype G[K any] struct{}\n\nfunc (g *G[K]) M(c chan int) {\n\tselect {\n\tcase <-c:\n\tdefault:\n\t}\n}\n"
	if fs, err := Functions("odd.go", odd); err != nil || len(fs) != 1 || fs[0].Name != "M" || fs[0].Complexity != 2 {
		t.Errorf("a declaration without a body is skipped; a generic receiver keeps the method name; select cases count: %+v %v", fs, err)
	}
	if _, err := Functions("bad.go", "package"); err == nil {
		t.Error("unparseable Go must fail")
	}
}

func TestTooComplex(t *testing.T) {
	limits := Limits{Complexity: 13, Lines: 80}
	got, err := TooComplex("a.go", before, after, limits)
	if err != nil || len(got) != 1 || !strings.Contains(got[0], "Grows") || !strings.Contains(got[0], "complexity 14") {
		t.Errorf("%q %v", got, err)
	}
	if got, _ := TooComplex("a.go", after, after, limits); len(got) != 0 {
		t.Errorf("a function already over the limit on base is not new: %q", got)
	}
	long := "package src\n\nfunc Long() {\n" + strings.Repeat("\t_ = 1\n", 85) + "}\n"
	if got, _ := TooComplex("l.go", "", long, limits); len(got) != 1 || !strings.Contains(got[0], "87 lines") {
		t.Errorf("a new long function: %q", got)
	}
	if _, err := TooComplex("a.go", "package", after, limits); err == nil {
		t.Error("an unparseable base must fail")
	}
	if _, err := TooComplex("a.go", before, "package", limits); err == nil {
		t.Error("an unparseable change must fail")
	}
}

func intsToStrings(n []int) []string {
	out := make([]string, len(n))
	for i, v := range n {
		out[i] = strconv.Itoa(v)
	}
	return out
}
