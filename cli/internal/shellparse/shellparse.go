// Package shellparse turns a shell command into the atomic commands it would run, so policy
// decides each one (doc 04 FR-SAFE-009). It never string-matches the raw text: lists, pipes,
// subshells, substitutions, loops, functions, in-command aliases and variables, wrappers
// (sudo, env, xargs …), `bash -c`, `eval`, here-documents fed to a shell, scripts and make
// targets are all expanded. Anything whose effect cannot be known statically is reported as
// unparseable (FR-SAFE-010: deny writes, approve reads).
package shellparse

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Word is one argument. Known is false when its value depends on something the parser cannot
// see (command output, unset variables, globs over unknown files …).
type Word struct {
	Value string
	Known bool
}

// Redirect is an output or input redirection of a command.
type Redirect struct {
	Op     string // ">", ">>", "<", "&>", "&>>", ">|", "<>"
	Target Word
}

// Command is one simple command the shell would run.
type Command struct {
	Args      []Word
	Assigns   map[string]Word // VAR=value prefixes (affect only this command)
	Redirects []Redirect
	Via       []string // wrappers it was unwrapped from, outermost first (sudo, xargs, bash -c …)
	Depth     int      // nesting through bash -c, eval, scripts, functions
	Source    string   // "" for the original command, or the script/Makefile it came from
	PipedIn   bool     // reads the previous command's output
}

// Name is the command name, or "" if it is not known.
func (c Command) Name() string {
	if len(c.Args) == 0 || !c.Args[0].Known {
		return ""
	}
	return c.Args[0].Value
}

// Result is everything a command line would run.
type Result struct {
	Commands    []Command
	Unparseable bool
	Reasons     []string
}

// Options controls parsing.
type Options struct {
	// ReadFile reads scripts and Makefiles referenced by the command (paths relative to Dir).
	// Nil means scripts are not followed and calls to them are reported as unresolved scripts.
	ReadFile func(path string) ([]byte, error)
	Dir      string
	MaxDepth int // default 5
}

// maxLoopItems bounds loop unrolling; longer loops are decided with the variable unknown.
const maxLoopItems = 32

// Parse analyses src.
func Parse(src string, opts Options) Result {
	if opts.MaxDepth == 0 {
		opts.MaxDepth = 5
	}
	p := &parser{opts: opts}
	p.parseSource(src, "", 0, scope{vars: map[string]Word{}, aliases: map[string]string{}, funcs: map[string]*syntax.Stmt{}})
	return p.res
}

type parser struct {
	opts Options
	res  Result
}

type scope struct {
	vars    map[string]Word
	aliases map[string]string
	funcs   map[string]*syntax.Stmt
}

func (p *parser) unparseable(format string, args ...any) {
	p.res.Unparseable = true
	p.res.Reasons = append(p.res.Reasons, fmt.Sprintf(format, args...))
}

func (p *parser) parseSource(src, source string, depth int, sc scope) {
	if depth > p.opts.MaxDepth {
		p.unparseable("nesting deeper than %d levels", p.opts.MaxDepth)
		return
	}
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if err != nil {
		p.unparseable("cannot parse%s: %v", where(source), err)
		return
	}
	w := &walker{p: p, sc: sc, depth: depth, source: source}
	w.stmts(f.Stmts, false)
}

func where(source string) string {
	if source == "" {
		return ""
	}
	return " " + source
}

type walker struct {
	p      *parser
	sc     scope
	depth  int
	source string
	via    []string
}

func (w *walker) stmts(list []*syntax.Stmt, piped bool) {
	for _, s := range list {
		w.stmt(s, piped)
	}
}

func (w *walker) stmt(s *syntax.Stmt, piped bool) {
	if s == nil {
		return
	}
	redirs, hdoc := w.redirects(s)
	switch c := s.Cmd.(type) {
	case *syntax.CallExpr:
		w.call(c, redirs, hdoc, piped)
	case *syntax.BinaryCmd:
		w.stmt(c.X, piped)
		w.stmt(c.Y, c.Op == syntax.Pipe || c.Op == syntax.PipeAll)
	case *syntax.Subshell:
		w.stmts(c.Stmts, piped)
	case *syntax.Block:
		w.stmts(c.Stmts, piped)
	case *syntax.TimeClause:
		w.stmt(c.Stmt, piped)
	case *syntax.FuncDecl:
		if c.Name == nil { // anonymous function: zsh runs it immediately, so it is decided now
			w.stmt(c.Body, piped)
			return
		}
		w.sc.funcs[c.Name.Value] = c.Body
	default:
		w.compound(c)
	}
}

// redirects collects output/input redirections and a here-document body, if any.
func (w *walker) redirects(s *syntax.Stmt) ([]Redirect, *syntax.Word) {
	var redirs []Redirect
	var hdoc *syntax.Word
	for _, r := range s.Redirs {
		w.wordSubst(r.Word)
		switch r.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			hdoc = r.Hdoc
		case syntax.WordHdoc:
			hdoc = r.Word
		default:
			if r.Word != nil {
				redirs = append(redirs, Redirect{Op: r.Op.String(), Target: w.word(r.Word)})
			}
		}
	}
	return redirs, hdoc
}

// compound walks control structures: every branch is decided, whichever would run.
func (w *walker) compound(cmd syntax.Command) {
	switch c := cmd.(type) {
	case *syntax.IfClause:
		for ic := c; ic != nil; ic = ic.Else {
			w.stmts(ic.Cond, false)
			w.stmts(ic.Then, false)
		}
	case *syntax.WhileClause:
		w.stmts(c.Cond, false)
		w.stmts(c.Do, false)
	case *syntax.ForClause:
		w.forClause(c)
	case *syntax.CaseClause:
		w.wordSubst(c.Word)
		for _, item := range c.Items {
			w.stmts(item.Stmts, false)
		}
	case *syntax.DeclClause:
		w.decl(c)
	case *syntax.TestClause, *syntax.ArithmCmd, *syntax.LetClause:
		syntax.Walk(c, w.substVisitor)
	default:
		w.p.unparseable("unsupported shell construct %T", c)
	}
}

// decl records export/declare/local/readonly/typeset as a command (some forms, like
// `export -p`, print every variable) and remembers the values they assign.
func (w *walker) decl(c *syntax.DeclClause) {
	args := []Word{{Value: c.Variant.Value, Known: true}}
	for _, a := range c.Args {
		switch {
		case a.Naked && a.Value != nil:
			args = append(args, w.word(a.Value))
		case a.Naked && a.Name != nil:
			args = append(args, Word{Value: a.Name.Value, Known: true})
		default:
			w.assign(a, w.sc.vars)
			v := w.sc.vars[a.Name.Value]
			args = append(args, Word{Value: a.Name.Value + "=" + v.Value, Known: v.Known})
		}
	}
	w.record(Command{Args: args, Assigns: map[string]Word{}, Via: append([]string(nil), w.via...), Depth: w.depth, Source: w.source})
}

func (w *walker) forClause(c *syntax.ForClause) {
	iter, ok := c.Loop.(*syntax.WordIter)
	if !ok {
		syntax.Walk(c.Loop, w.substVisitor)
		w.stmts(c.Do, false)
		return
	}
	var items []Word
	known := true
	for _, it := range iter.Items {
		v := w.word(it)
		known = known && v.Known
		items = append(items, fieldsOf(v)...)
	}
	if !known || len(items) == 0 || len(items) > maxLoopItems {
		w.sc.vars[iter.Name.Value] = Word{}
		w.stmts(c.Do, false)
		return
	}
	for _, it := range items {
		w.sc.vars[iter.Name.Value] = it
		w.stmts(c.Do, false)
	}
}

// fieldsOf splits a known unquoted value the way the shell would for a loop list.
func fieldsOf(v Word) []Word {
	if !v.Known {
		return []Word{v}
	}
	var out []Word
	for _, f := range strings.Fields(v.Value) {
		out = append(out, Word{Value: f, Known: true})
	}
	return out
}

func (w *walker) assign(a *syntax.Assign, into map[string]Word) {
	if a.Name == nil {
		return
	}
	switch {
	case a.Array != nil || a.Index != nil || a.Append:
		syntax.Walk(a, w.substVisitor)
		into[a.Name.Value] = Word{}
	case a.Value == nil:
		into[a.Name.Value] = Word{Value: "", Known: true}
	default:
		into[a.Name.Value] = w.word(a.Value)
	}
}

// substVisitor records commands inside $( ), ` ` and <( ) wherever they appear: they run too.
func (w *walker) substVisitor(n syntax.Node) bool {
	switch x := n.(type) {
	case *syntax.CmdSubst:
		w.stmts(x.Stmts, false)
		return false
	case *syntax.ProcSubst:
		w.stmts(x.Stmts, false)
		return false
	}
	return true
}

func (w *walker) wordSubst(word *syntax.Word) {
	if word != nil {
		syntax.Walk(word, w.substVisitor)
	}
}

// word evaluates a word as far as statically possible.
func (w *walker) word(word *syntax.Word) Word {
	w.wordSubst(word)
	var b strings.Builder
	known := true
	for _, part := range word.Parts {
		v, ok := w.part(part)
		known = known && ok
		b.WriteString(v)
	}
	val := b.String()
	if known && strings.ContainsAny(unquotedLits(word), "*?[") {
		known = false // a glob: the files it matches are not known statically
	}
	if strings.HasPrefix(val, "~/") || val == "~" {
		val = "$HOME" + strings.TrimPrefix(val, "~")
	}
	return Word{Value: val, Known: known}
}

func unquotedLits(word *syntax.Word) string {
	var b strings.Builder
	for _, part := range word.Parts {
		if l, ok := part.(*syntax.Lit); ok {
			b.WriteString(l.Value)
		}
	}
	return b.String()
}

func (w *walker) part(part syntax.WordPart) (string, bool) {
	switch x := part.(type) {
	case *syntax.Lit:
		return unescape(x.Value), true
	case *syntax.SglQuoted:
		if x.Dollar {
			return x.Value, false // $'…' escapes (e.g. \x72\x6d) can hide a command name
		}
		return x.Value, true
	case *syntax.DblQuoted:
		var b strings.Builder
		known := true
		for _, p := range x.Parts {
			v, ok := w.part(p)
			known = known && ok
			b.WriteString(v)
		}
		return b.String(), known
	case *syntax.ParamExp:
		return w.param(x)
	}
	return "", false // command/process substitution, arithmetic, extglob …
}

// param resolves $NAME / ${NAME}; any other expansion form is unknown.
func (w *walker) param(x *syntax.ParamExp) (string, bool) {
	simple := x.Param != nil && !x.Excl && !x.Length && x.Index == nil && x.Slice == nil &&
		x.Repl == nil && x.Exp == nil && x.NestedParam == nil
	if !simple {
		return "", false
	}
	v, ok := w.sc.vars[x.Param.Value]
	return v.Value, ok && v.Known
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (w *walker) call(c *syntax.CallExpr, redirs []Redirect, hdoc *syntax.Word, piped bool) {
	if len(c.Args) == 0 {
		for _, a := range c.Assigns {
			w.assign(a, w.sc.vars)
		}
		return
	}
	assigns := map[string]Word{}
	for _, a := range c.Assigns {
		w.assign(a, assigns)
	}
	args := make([]Word, 0, len(c.Args))
	for _, a := range c.Args {
		args = append(args, w.word(a))
	}
	cmd := Command{Args: args, Assigns: assigns, Redirects: redirs, Via: append([]string(nil), w.via...),
		Depth: w.depth, Source: w.source, PipedIn: piped}
	w.expand(cmd, hdoc)
}
