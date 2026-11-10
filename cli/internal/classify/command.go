package classify

import (
	"path"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/shellparse"
)

func classifyBash(c Call, pol *policy.Set, opts Options) Result {
	src := inputString(c.Input, "command")
	if strings.TrimSpace(src) == "" {
		return Result{Malformed: "Bash tool_input has no command"}
	}
	var read func(string) ([]byte, error)
	if opts.ReadFile != nil {
		read = func(p string) ([]byte, error) { return opts.ReadFile(absolute(c.Cwd, p)) }
	}
	parsed := shellparse.Parse(src, shellparse.Options{ReadFile: read, Dir: c.Cwd})
	res := Result{Unparseable: parsed.Unparseable, Reasons: parsed.Reasons}
	cl := &cmdClassifier{pol: pol, cwd: c.Cwd, read: opts.ReadFile}
	for _, cmd := range parsed.Commands {
		res.Actions = append(res.Actions, cl.redirects(cmd)...)
		a, unparseable := cl.command(cmd)
		a.Via = cmd.Via
		res.Actions = append(res.Actions, a)
		if unparseable != "" {
			res.Unparseable = true
			res.Reasons = append(res.Reasons, unparseable)
		}
	}
	if len(res.Actions) == 0 && !res.Unparseable {
		res.Actions = []Action{{Tool: "Bash", Tier: R0, Text: src}}
	}
	return res
}

type cmdClassifier struct {
	pol  *policy.Set
	cwd  string
	read func(string) ([]byte, error)
}

var outputOps = map[string]bool{">": true, ">>": true, "&>": true, "&>>": true, ">|": true, "<>": true}

var harmlessTargets = map[string]bool{"/dev/null": true, "/dev/stdout": true, "/dev/stderr": true, "/dev/tty": true}

// redirects turns > and < into file writes and reads.
func (cl *cmdClassifier) redirects(cmd shellparse.Command) []Action {
	var out []Action
	for _, r := range cmd.Redirects {
		target := pathOf(cl.cwd, r.Target)
		switch {
		case harmlessTargets[r.Target.Value] && r.Target.Known:
		case outputOps[r.Op]:
			a := Action{Tool: "redirect", Tier: W0, Text: r.Op + " " + target, Paths: []string{target}, Via: cmd.Via}
			out = append(out, stateFileGuard(a))
		case r.Op == "<" && r.Target.Known && IsSecretPath(r.Target.Value):
			out = append(out, Action{Tool: "redirect", Tier: X, Text: "< " + target, Forbidden: "reads a secret file (" + r.Target.Value + ")", Via: cmd.Via})
		case r.Op == "<":
			out = append(out, Action{Tool: "redirect", Tier: R0, Text: "< " + target, Paths: []string{target}, Via: cmd.Via})
		}
	}
	return out
}

// stateFileGuard forbids writing Athel's own state except through the athel CLI (doc 04 rule 7
// exception: .athel/changes/<id>/ "is written only through the helper CLI").
func stateFileGuard(a Action) Action {
	for _, p := range a.Paths {
		if strings.Contains("/"+p+"/", "/.athel/") {
			a.Tier, a.Forbidden = X, "edits Athel state directly ("+p+"); only the athel CLI may write .athel/"
		}
	}
	return a
}

// interpreters run code given inline; that code is not analysed.
var interpreters = map[string][]string{
	"python": {"-c"}, "python3": {"-c"}, "node": {"-e", "--eval", "-p"}, "ruby": {"-e"}, "perl": {"-e", "-E"},
	"php": {"-r"}, "osascript": {"-e"}, "awk": nil, "deno": {"eval"}, "bun": {"-e"},
}

// matcher classifies a command it recognises; ok is false otherwise.
type matcher func(cl *cmdClassifier, name string, cmd shellparse.Command) (a Action, unparseable string, ok bool)

// matchers are tried in order; the first that recognises the command decides it.
var matchers = []matcher{
	func(cl *cmdClassifier, name string, cmd shellparse.Command) (Action, string, bool) {
		switch {
		case name == "git":
			return cl.git(cmd), "", true
		case name == "gh":
			return cl.gh(cmd), "", true
		case name == "athel":
			return athelCall(cmd), "", true
		case cloudTools[name] != nil:
			return cl.cloud(name, cmd), "", true
		}
		return Action{}, "", false
	},
	func(_ *cmdClassifier, name string, cmd shellparse.Command) (Action, string, bool) {
		switch {
		case dbClients[name]:
			return dbClient(name, cmd), "", true
		case netTools[name]:
			return netCommand(name, cmd), "", true
		case remoteShells[name]:
			return remoteShell(name, cmd), "", true
		case printsEnvironment(name, cmd.Args):
			return Action{Tool: name, Tier: X, Text: text(cmd), Forbidden: "prints environment variables, which hold secrets"}, "", true
		}
		return Action{}, "", false
	},
	func(_ *cmdClassifier, name string, cmd shellparse.Command) (Action, string, bool) {
		if flags, ok := interpreters[name]; ok && hasAnyFlag(cmd.Args, flags) {
			return Action{Tool: name, Tier: W0, Text: text(cmd), Uncertain: UnknownCommand}, name + " runs inline code that is not analysed", true
		}
		a, ok := packageCommand(name, cmd)
		return a, "", ok
	},
	func(cl *cmdClassifier, _ string, cmd shellparse.Command) (Action, string, bool) {
		a, ok := fileCommand(cmd, cl.cwd)
		return stateFileGuard(a), "", ok
	},
}

// printsEnvironment: printenv, bare env/set, export -p, declare -p dump every variable.
func printsEnvironment(name string, args []shellparse.Word) bool {
	bare := len(args) == 1
	dashP := len(args) == 2 && args[1].Known && args[1].Value == "-p"
	return name == "printenv" || (bare && (name == "env" || name == "set")) || (dashP && (name == "export" || name == "declare"))
}

// command classifies one simple command. The second result is a reason when the command makes
// the whole call unparseable.
func (cl *cmdClassifier) command(cmd shellparse.Command) (Action, string) {
	if cmd.Name() == "" {
		return Action{Tool: "Bash", Tier: W0, Text: text(cmd), Uncertain: UnknownCommand}, ""
	}
	name := path.Base(cmd.Name())
	for _, m := range matchers {
		if a, unparseable, ok := m(cl, name, cmd); ok {
			return a, unparseable
		}
	}
	a := Action{Tool: name, Tier: W0, Text: text(cmd), Uncertain: UnknownCommand}
	if isLocalScript(cmd.Name()) || name == "make" || name == "gmake" {
		a.Uncertain = Unresolved
	}
	return a, ""
}

func isLocalScript(name string) bool {
	return strings.Contains(name, "/") && !strings.HasPrefix(name, "/usr/") && !strings.HasPrefix(name, "/bin/")
}

func hasAnyFlag(args []shellparse.Word, flags []string) bool {
	for _, a := range args[1:] {
		for _, f := range flags {
			if a.Known && (a.Value == f || strings.HasPrefix(a.Value, f+"=")) {
				return true
			}
		}
	}
	return false
}

// positionals returns non-flag words, skipping the values of flags listed in valueFlags
// (also --flag=value forms). It also returns those flag values by flag name.
func positionals(args []shellparse.Word, valueFlags map[string]bool) ([]shellparse.Word, map[string]shellparse.Word) {
	var pos []shellparse.Word
	vals := map[string]shellparse.Word{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !a.Known || !strings.HasPrefix(a.Value, "-") || a.Value == "-" {
			pos = append(pos, a)
			continue
		}
		if a.Value == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		flag, val, hasVal := strings.Cut(a.Value, "=")
		if hasVal {
			vals[flag] = shellparse.Word{Value: val, Known: true}
			continue
		}
		if valueFlags[flag] && i+1 < len(args) {
			vals[flag] = args[i+1]
			i++
			continue
		}
		vals[flag] = shellparse.Word{Value: "", Known: true}
	}
	return pos, vals
}

func hint(vals map[string]shellparse.Word, assigns map[string]shellparse.Word, flags []string, envVars []string) string {
	for _, f := range flags {
		if v, ok := vals[f]; ok {
			return known(v)
		}
	}
	for _, e := range envVars {
		if v, ok := assigns[e]; ok {
			return known(v)
		}
	}
	return ""
}

// flagVal is the value of a flag that may be absent ("" when absent, "?" when unknown).
func flagVal(vals map[string]shellparse.Word, flag string) string {
	v, ok := vals[flag]
	if !ok {
		return ""
	}
	return known(v)
}

func known(w shellparse.Word) string {
	if !w.Known {
		return "?"
	}
	return w.Value
}

func joinWords(ws []shellparse.Word) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = known(w)
	}
	return strings.Join(parts, " ")
}

// athelCall classifies the athel CLI itself. `athel hook` is reserved for Claude Code: if the
// model could call it, it could forge a human prompt (and with it a human-origin grant).
func athelCall(cmd shellparse.Command) Action {
	pos, vals := positionals(cmd.Args[1:], map[string]bool{"--to": true, "--by": true, "--reason": true, "--session": true,
		"--workspace": true, "--title": true, "--prd-file": true, "--prd-text": true, "--task": true, "--result": true,
		"--note": true, "--evidence": true, "--prompt-id": true, "-n": true, "--cost-usd": true, "--minutes": true,
		"--tool-calls": true, "--schema": true, "--layer": true})
	a := Action{Tool: "athel", Tier: R0, Text: text(cmd)}
	if len(pos) == 0 {
		return a
	}
	group := known(pos[0])
	sub := ""
	if len(pos) > 1 {
		sub = known(pos[1])
	}
	call := &AthelCall{Command: strings.TrimSpace(group + " " + sub)}
	a.Athel = call
	switch {
	case group == "hook":
		a.Tier, a.Forbidden = X, "athel hook is called only by Claude Code, never by a tool call"
		return a
	case athelReadGroups[group] || athelReads[call.Command]:
		return a
	default:
		humanOnly(call, pos, vals)
	}
	a.Tier = W0
	return a
}

// humanOnly marks the athel calls that act for the developer (doc 02 §Transition rules; M2 Q7;
// ADR 019) and binds each to what the developer's grant names.
func humanOnly(call *AthelCall, pos []shellparse.Word, vals map[string]shellparse.Word) {
	switch {
	case call.Command == "state transition" && len(pos) > 2:
		call.Change, call.To, call.ByHuman = known(pos[2]), flagVal(vals, "--to"), flagVal(vals, "--by") == "human"
	case call.Command == "repos roots" && len(pos) > 3 && known(pos[2]) == "set":
		// Confirmed roots widen rule 12, so saving them is the developer's act, like --by human.
		// shellparse writes a leading ~ as $HOME; the grant holds the ~ form the developer typed.
		roots := make([]string, 0, len(pos)-3)
		for _, w := range pos[3:] {
			v := known(w)
			if rest, ok := strings.CutPrefix(v, "$HOME/"); ok {
				v = "~/" + rest
			}
			roots = append(roots, v)
		}
		call.Command, call.Change, call.To, call.ByHuman = "repos roots set", grants.RootsKey(roots), grants.TargetRoots, true
	case call.Command == "blast signoff" && len(pos) > 2:
		call.Change, call.To, call.ByHuman = known(pos[2]), grants.TargetSignoff, true
	case call.Command == "decide answer" && len(pos) > 2:
		// Only the developer answers cards (ADR 019); the grant binds these exact answers.
		words := make([]string, 0, len(pos)-3)
		for _, w := range pos[3:] {
			words = append(words, known(w))
		}
		key := "decide:invalid"
		if answers, ok := grants.ParseAnswers(words); ok && len(answers) > 0 {
			key = grants.DecideKey(known(pos[2]), answers)
		}
		call.Change, call.To, call.ByHuman = key, grants.TargetDecide, true
	}
}

// athelReadGroups are athel command groups that only read; athelReads are single read-only
// subcommands of groups that also write. Every other athel call writes .athel/ (tier W0).
var (
	athelReadGroups = map[string]bool{"version": true, "status": true, "schema": true, "config": true, "policy": true, "audit": true, "playbook": true}
	athelReads      = map[string]bool{
		"change list": true, "change show": true, "state show": true,
		"lock status": true, "lock show": true, "checkpoint status": true, "checkpoint show": true,
		"budget status": true, "budget show": true, "fact find": true, "fact show": true,
		"investigation validate": true, "repos show": true, "ask show": true, "blast score": true,
		"prd show": true, "prd scan": true, "prd diff": true, "decide list": true, "decide show": true, "spec validate": true, "spec list": true, "spec show": true, "conventions show": true, "plan summary": true, "plan diff": true, "blast diff": true, "spec template": true, "decide template": true, "plan template": true, "prd review template": true,
	}
)
