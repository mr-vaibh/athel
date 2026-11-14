package gates

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Doc 06's coverage and complexity gates for Go (ADR 025), computed with the standard library.
// Coverage is measured on the changed lines, so deleting dead code never fails a task ("coverage
// drops because of deleted dead code" edge case); complexity counts only functions that are new
// or newly over the limits.

// MinChangedCoverage and MaxTotalDrop are doc 06's coverage criteria, in percent.
const (
	MinChangedCoverage = 90.0
	MaxTotalDrop       = 0.5
)

// block is one coverage block: lines start..end, its statements, and whether it ran.
type block struct {
	start, end, stmts int
	covered           bool
}

// Profile is a Go coverage profile, by file path relative to the repo.
type Profile struct {
	Blocks map[string][]block
}

var profileLine = regexp.MustCompile(`^(.+):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$`)

// ParseProfile reads `go test -coverprofile` output; module is the repo's module path, which
// the profile's file names start with.
func ParseProfile(text, module string) (Profile, error) {
	p := Profile{Blocks: map[string][]block{}}
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		m := profileLine.FindStringSubmatch(line)
		if m == nil {
			return p, errors.New("unreadable coverage profile line: " + line)
		}
		start, _ := strconv.Atoi(m[2]) // the pattern has digits only
		end, _ := strconv.Atoi(m[3])
		stmts, _ := strconv.Atoi(m[4])
		count, _ := strconv.Atoi(m[5])
		file := strings.TrimPrefix(strings.TrimPrefix(m[1], module), "/")
		p.Blocks[file] = append(p.Blocks[file], block{start: start, end: end, stmts: stmts, covered: count > 0})
	}
	return p, nil
}

// Total is the share of statements covered, in percent (100 with no statements).
func (p Profile) Total() float64 {
	var all, hit int
	for _, bs := range p.Blocks {
		for _, b := range bs {
			all += b.stmts
			if b.covered {
				hit += b.stmts
			}
		}
	}
	if all == 0 {
		return 100
	}
	return 100 * float64(hit) / float64(all)
}

// ChangedCoverage counts changed lines inside covered and uncovered blocks; lines in no block
// (comments, declarations) do not count.
func (p Profile) ChangedCoverage(changed map[string][]int) (int, int) {
	var covered, uncovered int
	for file, lines := range changed {
		for _, n := range lines {
			in, hit := false, false
			for _, b := range p.Blocks[file] {
				if n >= b.start && n <= b.end {
					in, hit = true, hit || b.covered
				}
			}
			switch {
			case hit:
				covered++
			case in:
				uncovered++
			}
		}
	}
	return covered, uncovered
}

var hunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ChangedLines reads `git diff -U0` and lists the new line numbers of every changed Go file
// that is not a test.
func ChangedLines(diff string) map[string][]int {
	out := map[string][]int{}
	file := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
				file = ""
			}
		case file != "" && hunk.MatchString(line):
			m := hunk.FindStringSubmatch(line)
			start, _ := strconv.Atoi(m[1]) // digits only
			n := 1
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			}
			for i := range n {
				out[file] = append(out[file], start+i)
			}
		}
	}
	return out
}

// Function is one function's size: its name (Type.Method for methods), complexity and lines.
type Function struct {
	Name       string
	Complexity int
	Lines      int
}

// Functions measures every function in a Go file: cyclomatic complexity as gocyclo counts it
// (1, plus each if, for, range, non-default case, && and ||) and its length in lines.
func Functions(name, src string) ([]Function, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []Function
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		out = append(out, Function{Name: funcName(fn), Complexity: complexity(fn),
			Lines: fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1})
	}
	return out, nil
}

func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func complexity(fn *ast.FuncDecl) int {
	c := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			c++
		case *ast.CaseClause:
			if n.List != nil {
				c++
			}
		case *ast.CommClause:
			if n.Comm != nil {
				c++
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				c++
			}
		}
		return true
	})
	return c
}

// Limits are doc 06's complexity criteria.
type Limits struct {
	Complexity int // over this fails
	Lines      int // longer than this fails
}

// DefaultLimits are doc 06's: complexity over 15, or longer than 80 lines.
var DefaultLimits = Limits{Complexity: 15, Lines: 80}

// TooComplex lists the functions of a file that are over the limits after the change and were
// not before (new functions, or ones the change pushed over). base is "" for a new file.
func TooComplex(name, base, now string, l Limits) ([]string, error) {
	was := map[string]Function{}
	if base != "" {
		fs, err := Functions(name, base)
		if err != nil {
			return nil, err
		}
		for _, f := range fs {
			was[f.Name] = f
		}
	}
	fs, err := Functions(name, now)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range fs {
		old, existed := was[f.Name]
		if f.Complexity > l.Complexity && (!existed || old.Complexity <= l.Complexity) {
			out = append(out, fmt.Sprintf("%s: %s has complexity %d (limit %d)", name, f.Name, f.Complexity, l.Complexity))
		}
		if f.Lines > l.Lines && (!existed || old.Lines <= l.Lines) {
			out = append(out, fmt.Sprintf("%s: %s is %d lines long (limit %d)", name, f.Name, f.Lines, l.Lines))
		}
	}
	sort.Strings(out)
	return out, nil
}
