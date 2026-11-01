package shellparse

import (
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// wrapperFlagsWithValue lists, per wrapper, the flags that consume the next argument.
var wrapperFlagsWithValue = map[string]map[string]bool{
	"sudo":    {"-u": true, "-g": true, "-h": true, "-p": true, "-C": true, "-r": true, "-t": true, "-U": true, "-D": true, "-R": true},
	"doas":    {"-u": true, "-C": true},
	"env":     {"-u": true, "-C": true, "--unset": true, "--chdir": true},
	"nice":    {"-n": true, "--adjustment": true},
	"ionice":  {"-c": true, "-n": true, "-p": true},
	"timeout": {"-s": true, "-k": true, "--signal": true, "--kill-after": true},
	"stdbuf":  {"-i": true, "-o": true, "-e": true},
	"exec":    {"-a": true},
	"xargs":   {"-I": true, "-i": false, "-n": true, "-P": true, "-L": true, "-d": true, "-E": true, "-s": true, "-a": true, "--max-args": true, "--max-procs": true, "--delimiter": true, "--arg-file": true, "--replace": true},
	"watch":   {"-n": true, "-d": false, "--interval": true},
}

// simpleWrappers run the rest of their arguments as a command after optional flags.
var simpleWrappers = map[string]bool{
	"sudo": true, "doas": true, "env": true, "nohup": true, "time": true, "nice": true, "ionice": true,
	"timeout": true, "stdbuf": true, "command": true, "builtin": true, "exec": true, "xargs": true,
	"caffeinate": true, "unbuffer": true, "setsid": true, "chronic": true, "watch": true, "flock": true,
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true}

func (w *walker) record(c Command) { w.p.res.Commands = append(w.p.res.Commands, c) }

func (w *walker) expand(c Command, hdoc *syntax.Word) {
	name := c.Name()
	if name == "" {
		w.record(c)
		w.p.unparseable("a command name is computed at run time")
		return
	}
	base := path.Base(name)
	switch {
	case base == "alias":
		w.defineAliases(c)
		w.record(c)
	case w.sc.aliases[name] != "":
		w.expandAlias(c)
	case w.sc.funcs[name] != nil:
		w.callFunc(c)
	case simpleWrappers[base]:
		w.unwrap(c, base, hdoc)
	case shells[base]:
		w.shell(c, base, hdoc)
	case base == "eval":
		w.inline(c, c.Args[1:], "eval")
	case base == "source" || base == ".":
		w.record(c)
		if len(c.Args) > 1 {
			w.script(c.Args[1], "source")
		}
	case base == "make" || base == "gmake":
		w.make(c)
	case isScriptPath(name) && w.script(c.Args[0], ""):
	default:
		w.record(c)
	}
}

// isScriptPath reports whether name is a path to a local script rather than a system binary.
func isScriptPath(name string) bool {
	if !strings.Contains(name, "/") {
		return false
	}
	for _, sys := range []string{"/usr/", "/bin/", "/opt/", "/sbin/"} {
		if strings.HasPrefix(name, sys) {
			return false
		}
	}
	return true
}

func (w *walker) defineAliases(c Command) {
	for _, a := range c.Args[1:] {
		k, v, ok := strings.Cut(a.Value, "=")
		if !ok {
			continue
		}
		if !a.Known {
			w.p.unparseable("alias %s has a value computed at run time", k)
			continue
		}
		w.sc.aliases[k] = v
	}
}

func (w *walker) expandAlias(c Command) {
	src := w.sc.aliases[c.Name()]
	for _, a := range c.Args[1:] {
		src += " " + quote(a)
	}
	delete(w.sc.aliases, c.Name()) // the shell does not expand an alias inside itself
	w.parseNested(src, "alias "+c.Name(), c)
}

func (w *walker) callFunc(c Command) {
	if w.depth+1 > w.p.opts.MaxDepth {
		w.p.unparseable("function %s nests deeper than %d levels", c.Name(), w.p.opts.MaxDepth)
		return
	}
	sub := &walker{p: w.p, sc: w.sc, depth: w.depth + 1, source: w.source, via: append(append([]string(nil), c.Via...), "function "+c.Name())}
	sub.stmt(w.sc.funcs[c.Name()], c.PipedIn)
}

func quote(a Word) string {
	if !a.Known {
		return "$ATHEL_UNKNOWN_VALUE"
	}
	q, err := syntax.Quote(a.Value, syntax.LangBash)
	if err != nil {
		return "$ATHEL_UNKNOWN_VALUE"
	}
	return q
}

// unwrap strips a wrapper and its flags and expands the command it runs.
func (w *walker) unwrap(c Command, base string, hdoc *syntax.Word) {
	assigns := map[string]Word{}
	for k, v := range c.Assigns {
		assigns[k] = v
	}
	rest, split := skipWrapperFlags(base, c.Args[1:], assigns)
	switch {
	case split != nil:
		w.inline(c, split, "env -S")
		return
	case base == "watch":
		w.inline(c, rest, "watch")
		return
	case base == "timeout" && len(rest) > 0:
		rest = rest[1:] // the duration
	}
	if len(rest) == 0 {
		w.record(c)
		return
	}
	if base == "xargs" {
		rest = append(append([]Word(nil), rest...), Word{}) // input lines become unknown arguments
	}
	inner := Command{Args: rest, Assigns: assigns, Redirects: c.Redirects, Via: append(append([]string(nil), c.Via...), base),
		Depth: c.Depth, Source: c.Source, PipedIn: c.PipedIn}
	w.expand(inner, hdoc)
}

func isEnvAssign(base string, a Word) bool {
	return base == "env" && a.Known && strings.Contains(a.Value, "=") && !strings.HasPrefix(a.Value, "-")
}

func isEnvSplit(base, flag string) bool {
	return base == "env" && (flag == "-S" || flag == "--split-string")
}

// skipWrapperFlags drops a wrapper's flags (and their values) and env's VAR=value arguments,
// returning the wrapped command, or the words of `env -S "…"` to parse as shell.
func skipWrapperFlags(base string, rest []Word, assigns map[string]Word) ([]Word, []Word) {
	takesValue := wrapperFlagsWithValue[base]
	for len(rest) > 0 {
		a := rest[0]
		switch {
		case a.Known && a.Value == "--":
			return rest[1:], nil
		case isEnvAssign(base, a):
			k, v, _ := strings.Cut(a.Value, "=")
			assigns[k] = Word{Value: v, Known: true}
			rest = rest[1:]
			continue
		case !a.Known || !strings.HasPrefix(a.Value, "-"):
			return rest, nil
		}
		flag, _, inline := strings.Cut(a.Value, "=")
		rest = rest[1:]
		if isEnvSplit(base, flag) && len(rest) > 0 {
			return nil, rest
		}
		if takesValue[flag] && !inline && len(rest) > 0 {
			rest = rest[1:]
		}
	}
	return rest, nil
}

// inline parses the words as shell source (eval, env -S, watch).
func (w *walker) inline(c Command, words []Word, via string) {
	var parts []string
	for _, a := range words {
		if !a.Known {
			w.record(c)
			w.p.unparseable("%s runs text computed at run time", via)
			return
		}
		parts = append(parts, a.Value)
	}
	w.parseNested(strings.Join(parts, " "), via, c)
}

func (w *walker) parseNested(src, via string, c Command) {
	if w.depth+1 > w.p.opts.MaxDepth {
		w.p.unparseable("%s nests deeper than %d levels", via, w.p.opts.MaxDepth)
		return
	}
	before := len(w.p.res.Commands)
	w.p.parseSource(src, w.source, w.depth+1, w.sc)
	for i := before; i < len(w.p.res.Commands); i++ {
		cmd := &w.p.res.Commands[i]
		cmd.Via = append(append(append([]string(nil), c.Via...), via), cmd.Via...)
		if len(c.Redirects) > 0 {
			cmd.Redirects = append(cmd.Redirects, c.Redirects...)
		}
		for k, v := range c.Assigns {
			if _, set := cmd.Assigns[k]; !set {
				cmd.Assigns[k] = v
			}
		}
	}
}

// shell handles sh/bash/zsh invocations: -c strings, script files, here-documents and piped
// input. Anything else is unparseable.
func (w *walker) shell(c Command, base string, hdoc *syntax.Word) {
	args := c.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Known {
			w.record(c)
			w.p.unparseable("%s is started with arguments computed at run time", base)
			return
		}
		switch {
		case a.Value == "-c" || (strings.HasPrefix(a.Value, "-") && !strings.HasPrefix(a.Value, "--") && strings.Contains(a.Value, "c")):
			if i+1 >= len(args) {
				w.record(c)
				w.p.unparseable("%s -c without a command", base)
				return
			}
			w.inline(c, args[i+1:i+2], base+" -c")
			return
		case a.Value == "-s" || strings.HasPrefix(a.Value, "-"):
			continue
		default:
			if !w.script(a, base) {
				w.record(c)
			}
			return
		}
	}
	if hdoc != nil {
		body := w.word(hdoc)
		if body.Known {
			w.parseNested(body.Value, base+" <<heredoc", c)
			return
		}
	}
	w.record(c)
	if c.PipedIn || hdoc != nil {
		w.p.unparseable("commands are fed to %s on standard input", base)
		return
	}
	w.p.unparseable("an interactive %s cannot be analysed", base)
}

// script follows a script file. It reports whether the script was read and parsed; an
// unreadable script is left to the classifier as an unresolved script (approve). `source` runs
// in the current shell, so it shares the caller's variables, aliases and functions; any other
// script starts with an empty scope.
func (w *walker) script(file Word, via string) bool {
	if !file.Known || w.p.opts.ReadFile == nil {
		return false
	}
	src, err := w.p.opts.ReadFile(file.Value)
	if err != nil {
		return false
	}
	if first, _, _ := strings.Cut(string(src), "\n"); strings.HasPrefix(first, "#!") && !shebangIsShell(first) {
		return false
	}
	label := "script " + file.Value
	if via != "" {
		label = via + " " + file.Value
	}
	if w.depth+1 > w.p.opts.MaxDepth {
		w.p.unparseable("%s nests deeper than %d levels", label, w.p.opts.MaxDepth)
		return true
	}
	sc := scope{vars: map[string]Word{}, aliases: map[string]string{}, funcs: map[string]*syntax.Stmt{}}
	if via == "source" {
		sc = w.sc
	}
	before := len(w.p.res.Commands)
	w.p.parseSource(string(src), file.Value, w.depth+1, sc)
	for i := before; i < len(w.p.res.Commands); i++ {
		w.p.res.Commands[i].Via = append([]string{label}, w.p.res.Commands[i].Via...)
	}
	return true
}

func shebangIsShell(line string) bool {
	for s := range shells {
		if strings.HasSuffix(line, "/"+s) || strings.Contains(line, "/"+s+" ") || strings.HasSuffix(line, "env "+s) {
			return true
		}
	}
	return false
}

var makeVarRe = regexp.MustCompile(`\$[({]([A-Za-z_][A-Za-z0-9_]*)[)}]`)

// make expands the recipes of the requested targets from the Makefile.
func (w *walker) make(c Command) {
	file, targets, ok := makeArgs(c.Args[1:])
	if !ok || w.p.opts.ReadFile == nil {
		w.record(c)
		return
	}
	src, err := w.p.opts.ReadFile(file)
	if err != nil {
		w.record(c)
		return
	}
	recipes, vars, first := parseMakefile(string(src))
	if len(targets) == 0 && first != "" {
		targets = []string{first}
	}
	for _, t := range targets {
		recipe, ok := recipes[t]
		if !ok {
			w.record(c)
			continue
		}
		expanded := makeVarRe.ReplaceAllStringFunc(recipe, func(m string) string {
			if v, ok := vars[makeVarRe.FindStringSubmatch(m)[1]]; ok {
				return v
			}
			return "$ATHEL_UNKNOWN_VALUE"
		})
		w.parseNested(strings.ReplaceAll(expanded, "$$", "$"), "make "+t, c)
	}
}

// makeArgs returns the Makefile path and targets; ok is false if any argument is unknown.
func makeArgs(args []Word) (string, []string, bool) {
	file, dir := "Makefile", ""
	var targets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case !a.Known:
			return "", nil, false
		case (a.Value == "-f" || a.Value == "-C") && i+1 < len(args):
			if a.Value == "-f" {
				file = args[i+1].Value
			} else {
				dir = args[i+1].Value
			}
			i++
		case strings.HasPrefix(a.Value, "-") || strings.Contains(a.Value, "="):
		default:
			targets = append(targets, a.Value)
		}
	}
	return path.Join(dir, file), targets, true
}

var (
	makeTargetRe = regexp.MustCompile(`^([A-Za-z0-9_./\-]+)\s*:([^=]|$)`)
	makeAssignRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*[:?+]?=\s*(.*)$`)
)

// parseMakefile extracts simple variables and the recipe of each explicit target. Recipes of
// other targets they depend on are not followed: a prerequisite target is analysed only if the
// command names it (kept simple on purpose; an unknown target is an unresolved make call).
func parseMakefile(src string) (map[string]string, map[string]string, string) {
	recipes, vars := map[string]string{}, map[string]string{}
	first, current := "", ""
	for _, line := range strings.Split(strings.ReplaceAll(src, "\\\n", " "), "\n") {
		switch {
		case strings.HasPrefix(line, "\t") && current != "":
			cmd := strings.TrimLeft(strings.TrimSpace(line), "@-+")
			recipes[current] += cmd + "\n"
		case makeAssignRe.MatchString(line):
			m := makeAssignRe.FindStringSubmatch(line)
			vars[m[1]] = strings.TrimSpace(m[2])
			current = ""
		case makeTargetRe.MatchString(line):
			current = makeTargetRe.FindStringSubmatch(line)[1]
			if _, ok := recipes[current]; !ok {
				recipes[current] = ""
			}
			if first == "" && !strings.HasPrefix(current, ".") {
				first = current
			}
		default:
			current = ""
		}
	}
	return recipes, vars, first
}
