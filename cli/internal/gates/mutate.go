package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
)

// The mutation gate for HIGH-risk tasks (doc 06; ADR 025): small, deterministic changes to the
// changed lines of Go code, each of which the tests should notice. A mutant the tests still pass
// with marks a behaviour no test pins down.

// MinMutationScore is doc 06's bar, in percent of mutants killed.
const MinMutationScore = 70.0

// MaxMutants bounds a run: each mutant costs one test run of its package.
const MaxMutants = 50

// Mutant is one mutated version of a file.
type Mutant struct {
	File string
	Line int
	Desc string
	Src  string // the whole mutated file
}

// swaps are the operator mutations: comparisons, arithmetic and boolean logic.
var swaps = map[token.Token]token.Token{
	token.EQL: token.NEQ, token.NEQ: token.EQL, token.LSS: token.LEQ, token.LEQ: token.LSS,
	token.GTR: token.GEQ, token.GEQ: token.GTR, token.ADD: token.SUB, token.SUB: token.ADD,
	token.LAND: token.LOR, token.LOR: token.LAND,
}

// Mutants lists the mutants of a file's changed lines, in source order: swapped operators,
// flipped booleans, and dropped `if err != nil` returns.
func Mutants(file, src string, lines []int) ([]Mutant, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	changed := func(p token.Pos) bool { return slices.Contains(lines, fset.Position(p).Line) }
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	var out []Mutant
	add := func(at token.Pos, start, end int, repl, desc string) {
		line := fset.Position(at).Line
		out = append(out, Mutant{File: file, Line: line, Desc: fmt.Sprintf("line %d: %s", line, desc), Src: src[:start] + repl + src[end:]})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if to, ok := swaps[n.Op]; ok && changed(n.OpPos) {
				add(n.OpPos, off(n.OpPos), off(n.OpPos)+len(n.Op.String()), to.String(), n.Op.String()+" to "+to.String())
			}
		case *ast.Ident:
			if flip, ok := map[string]string{"true": "false", "false": "true"}[n.Name]; ok && changed(n.Pos()) {
				add(n.Pos(), off(n.Pos()), off(n.End()), flip, n.Name+" to "+flip)
			}
		case *ast.IfStmt:
			if isErrCheck(n.Cond) && returns(n.Body) && changed(n.Pos()) {
				add(n.Pos(), off(n.Body.Lbrace), off(n.Body.Rbrace)+1, "{\n}", "drop the err != nil return")
			}
		}
		return true
	})
	return out, nil
}

// isErrCheck matches `err != nil`.
func isErrCheck(e ast.Expr) bool {
	b, ok := e.(*ast.BinaryExpr)
	if !ok || b.Op != token.NEQ {
		return false
	}
	x, xok := b.X.(*ast.Ident)
	y, yok := b.Y.(*ast.Ident)
	return xok && yok && x.Name == "err" && y.Name == "nil"
}

func returns(b *ast.BlockStmt) bool {
	return slices.ContainsFunc(b.List, func(s ast.Stmt) bool { _, ok := s.(*ast.ReturnStmt); return ok })
}

// Cap keeps the first MaxMutants, so a run's cost is bounded and its choice deterministic.
func Cap(ms []Mutant) []Mutant {
	if len(ms) > MaxMutants {
		return ms[:MaxMutants]
	}
	return ms
}

// Score is the share of mutants killed, in percent; 100 with none.
func Score(killed, survived int) float64 {
	if killed+survived == 0 {
		return 100
	}
	return 100 * float64(killed) / float64(killed+survived)
}
