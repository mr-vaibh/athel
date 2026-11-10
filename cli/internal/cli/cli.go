// Package cli is the athel command line: argument parsing, dispatch, the --json envelope and
// exit codes. It holds no business logic; commands call the state, config and hook packages.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Env is everything the CLI reads from the outside world, injected so tests are hermetic.
type Env struct {
	Args     []string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Getenv   func(string) string
	Cwd      string
	UserHome func() (string, error)
	Clock    clock.Clock
	Version  string
	// ClaudeVersion reports the running Claude Code version (SessionStart compatibility check).
	ClaudeVersion func() (string, error)
}

// usageError is a wrong invocation: exit 2, never an Athel error code.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return &usageError{msg: fmt.Sprintf(format, args...)} }

// helpError is -h or --help on a command: its flags are printed and the exit code is 0, so
// a caller (the scout) can learn a command instead of guessing its flags.
type helpError struct{ text string }

func (h *helpError) Error() string { return h.text }

// command runs with the remaining arguments and returns the data to print.
type command func(r *runCtx, args []string) (any, error)

// commands maps "group" or "group sub" to its implementation. Filled by the per-area files.
func commands() map[string]command {
	return map[string]command{
		"version":                cmdVersion,
		"init":                   cmdInit,
		"change new":             cmdChangeNew,
		"change list":            cmdChangeList,
		"change show":            cmdChangeShow,
		"state show":             cmdStateShow,
		"state transition":       cmdStateTransition,
		"state enter":            cmdStateEnter,
		"state exit":             cmdStateExit,
		"checkpoint set":         cmdCheckpointSet,
		"checkpoint show":        cmdCheckpointShow,
		"lock acquire":           cmdLockAcquire,
		"lock refresh":           cmdLockRefresh,
		"lock release":           cmdLockRelease,
		"lock status":            cmdLockStatus,
		"budget add":             cmdBudgetAdd,
		"budget show":            cmdBudgetShow,
		"resume":                 cmdResume,
		"config validate":        cmdConfigValidate,
		"config show":            cmdConfigShow,
		"schema validate":        cmdSchemaValidate,
		"hook":                   cmdHook,
		"audit verify":           cmdAuditVerify,
		"audit tail":             cmdAuditTail,
		"policy validate":        cmdPolicyValidate,
		"policy show":            cmdPolicyShow,
		"policy decide":          cmdPolicyDecide,
		"plan set":               cmdPlanSet,
		"plan show":              cmdPlanShow,
		"plan summary":           cmdPlanSummary,
		"plan diff":              cmdPlanDiff,
		"trace":                  cmdTrace,
		"migrate":                cmdMigrate,
		"integrity accept":       cmdIntegrityAccept,
		"fact add":               cmdFactAdd,
		"fact find":              cmdFactFind,
		"fact show":              cmdFactShow,
		"fact use":               cmdFactUse,
		"fact verify":            cmdFactVerify,
		"investigation validate": cmdInvestigationValidate,
		"investigation save":     cmdInvestigationSave,
		"investigation template": cmdInvestigationTemplate,
		"repos roots propose":    cmdReposRootsPropose,
		"repos roots set":        cmdReposRootsSet,
		"repos index":            cmdReposIndex,
		"repos show":             cmdReposShow,
		"repos cache":            cmdReposCache,
		"ask new":                cmdAskNew,
		"ask show":               cmdAskShow,
		"playbook list":          cmdPlaybookList,
		"playbook show":          cmdPlaybookShow,
		"blast score":            cmdBlastScore,
		"blast save":             cmdBlastSave,
		"fact archive":           cmdFactArchive,
		"prd index":              cmdPRDIndex,
		"prd show":               cmdPRDShow,
		"prd scan":               cmdPRDScan,
		"prd add-version":        cmdPRDAddVersion,
		"prd diff":               cmdPRDDiff,
		"prd review save":        cmdPRDReviewSave,
		"decide add":             cmdDecideAdd,
		"decide record":          cmdDecideRecord,
		"decide list":            cmdDecideList,
		"decide show":            cmdDecideShow,
		"decide present":         cmdDecidePresent,
		"decide answer":          cmdDecideAnswer,
		"decide tick":            cmdDecideTick,
		"spec validate":          cmdSpecValidate,
		"spec save":              cmdSpecSave,
		"spec list":              cmdSpecList,
		"spec show":              cmdSpecShow,
		"repos survey":           cmdReposSurvey,
		"conventions save":       cmdConventionsSave,
		"conventions show":       cmdConventionsShow,
	}
}

// Run executes one athel invocation and returns the process exit code.
func Run(env Env) int {
	r, rest, err := parseGlobals(env)
	if err != nil {
		return report(env, r.json, nil, err)
	}
	cmd, args, err := resolve(rest)
	if err != nil {
		return report(env, r.json, nil, err)
	}
	data, err := cmd(r, args)
	return report(env, r.json, data, err)
}

func parseGlobals(env Env) (*runCtx, []string, error) {
	r := &runCtx{env: env}
	fs := flag.NewFlagSet("athel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&r.json, "json", false, "print one JSON object")
	fs.StringVar(&r.workspace, "workspace", "", "workspace root (default: nearest parent with .athel/)")
	fs.StringVar(&r.session, "session", "", "Claude Code session ID (default: $ATHEL_SESSION_ID)")
	if err := fs.Parse(env.Args); err != nil {
		return r, nil, usagef("%v", err)
	}
	if r.session == "" {
		r.session = env.Getenv("ATHEL_SESSION_ID")
	}
	return r, fs.Args(), nil
}

// resolve finds the longest command name (up to three words) the arguments start with.
func resolve(args []string) (command, []string, error) {
	cmds := commands()
	for n := min(3, len(args)); n >= 1; n-- {
		if c, ok := cmds[strings.Join(args[:n], " ")]; ok {
			return c, args[n:], nil
		}
	}
	names := make([]string, 0, len(cmds))
	for n := range cmds {
		names = append(names, n)
	}
	sort.Strings(names)
	return nil, nil, usagef("usage: athel [--json] [--workspace DIR] [--session ID] <command>\ncommands: %s", strings.Join(names, ", "))
}

// envelope is the --json output shape.
type envelope struct {
	OK    bool `json:"ok"`
	Data  any  `json:"data,omitempty"`
	Error any  `json:"error,omitempty"`
}

type usageJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func report(env Env, asJSON bool, data any, err error) int {
	var help *helpError
	if errors.As(err, &help) {
		return writeOrInternal(env.Stdout, help.text, aerr.ExitOK)
	}
	if raw, ok := data.(rawOutput); ok && err == nil {
		return writeOrInternal(env.Stdout, string(raw)+"\n", aerr.ExitOK)
	}
	code := exitCode(err)
	if asJSON {
		out := envelope{OK: err == nil, Data: data}
		if err != nil {
			out.Data = nil
			out.Error = jsonError(err)
		}
		b, mErr := json.Marshal(out)
		if mErr != nil {
			return writeOrInternal(env.Stderr, "error: encode output: "+mErr.Error()+"\n", aerr.ExitInternal)
		}
		return writeOrInternal(env.Stdout, string(b)+"\n", code)
	}
	if err != nil {
		return writeOrInternal(env.Stderr, humanError(err), code)
	}
	text, tErr := human(data)
	if tErr != nil {
		return writeOrInternal(env.Stderr, "error: render output: "+tErr.Error()+"\n", aerr.ExitInternal)
	}
	return writeOrInternal(env.Stdout, text, code)
}

func exitCode(err error) int {
	var u *usageError
	if errors.As(err, &u) {
		return aerr.ExitUsage
	}
	return aerr.ExitCode(err)
}

func jsonError(err error) any {
	var u *usageError
	if errors.As(err, &u) {
		return usageJSON{Code: "USAGE", Message: u.msg}
	}
	if e, ok := aerr.As(err); ok {
		if joined := allMessages(err); joined != e.Error() {
			copied := *e
			copied.Message = strings.TrimPrefix(joined, string(e.Code)+": ")
			return &copied
		}
		return e
	}
	return usageJSON{Code: "INTERNAL", Message: err.Error()}
}

// allMessages flattens errors.Join so a secondary failure (for example the audit write) is
// never hidden behind the primary one.
func allMessages(err error) string {
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}

func humanError(err error) string {
	var b strings.Builder
	b.WriteString("error: " + allMessages(err) + "\n")
	if e, ok := aerr.As(err); ok && e.NextAction != "" {
		b.WriteString("next: " + e.NextAction + "\n")
	}
	return b.String()
}

// human renders data as YAML: readable, and the same keys as the --json output.
func human(data any) (string, error) {
	if data == nil {
		return "", nil
	}
	if s, ok := data.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	var plain any
	if err := json.Unmarshal(b, &plain); err != nil {
		return "", err
	}
	out, err := yaml.Marshal(plain)
	return string(out), err
}

func writeOrInternal(w io.Writer, s string, code int) int {
	if _, err := io.WriteString(w, s); err != nil {
		return aerr.ExitInternal
	}
	return code
}

// oneOf returns the single non-empty value, for mutually exclusive flags.
func oneOf(names []string, values []bool) (string, error) {
	var set []string
	for i, v := range values {
		if v {
			set = append(set, names[i])
		}
	}
	if len(set) != 1 {
		return "", usagef("exactly one of %s is required", strings.Join(names, ", "))
	}
	return set[0], nil
}

// newFlags returns a flag set for one command that also accepts the global flags, so
// `athel state show --json` works as well as `athel --json state show`.
func (r *runCtx) newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&r.json, "json", r.json, "print one JSON object")
	fs.StringVar(&r.workspace, "workspace", r.workspace, "workspace root")
	fs.StringVar(&r.session, "session", r.session, "Claude Code session ID")
	return fs
}

// helpFor lists a command's flags in the flag package's own format.
func helpFor(fs *flag.FlagSet) error {
	var b strings.Builder
	b.WriteString("usage: athel " + fs.Name() + " [flags] [arguments]\nflags:\n")
	fs.SetOutput(&b)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
	return &helpError{text: b.String()}
}

// parse parses flags and positional arguments in any order, requiring exactly wantArgs
// positional arguments (-1: any number).
func parse(fs *flag.FlagSet, args []string, wantArgs int) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, helpFor(fs)
			}
			return nil, usagef("%s: %v", fs.Name(), err)
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if wantArgs >= 0 && len(positional) != wantArgs {
		return nil, usagef("%s: expected %d argument(s), got %d", fs.Name(), wantArgs, len(positional))
	}
	return positional, nil
}
