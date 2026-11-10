package facts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/observe"
)

// Outcomes of one recipe step.
const (
	Match       = "match"
	Mismatch    = "mismatch"
	Unreachable = "unreachable" // the source could not be read; says nothing about the fact
)

// Expectations a step can state; present is the default.
const (
	ExpectPresent = "present"
	ExpectAbsent  = "absent"
)

// StepResult is the outcome of one recipe step.
type StepResult struct {
	Step    int    `json:"step"`
	Tool    string `json:"tool"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Detail  string `json:"detail"`
}

// Recipe checks recipe steps (doc 03 §Verification rules). Git steps run here, read-only, on
// committed content; every other tool's step is judged from the output the PostToolUse hook
// captured for the call the scout made, never from what the model says it saw (M2 plan G2).
type Recipe struct {
	Repo        func(name string) (string, error)                     // repo name → local or cache clone
	Observation func(toolUseID string) (observe.Observation, error)   // captured scout calls
	ToolsOf     func(tool string, input []byte) []string              // tools.yaml ids of a captured call
	Observed    map[int]string                                        // step index → tool_use_id
	Git         func(dir string, args ...string) (int, string, error) // runs git; nil means the real one
}

// Check runs step i.
func (r Recipe) Check(step Step, i int) StepResult {
	res := StepResult{Step: i, Tool: str(step["tool"]), Action: str(step["action"])}
	expect := str(step["expect"])
	if expect == "" {
		expect = ExpectPresent
	}
	if expect != ExpectPresent && expect != ExpectAbsent {
		return res.with(Unreachable, "unsupported expect "+expect+"; use present or absent")
	}
	if res.Tool == "git" {
		return r.git(step, res, expect)
	}
	return r.observed(step, res, expect, i)
}

func (res StepResult) with(outcome, detail string) StepResult {
	res.Outcome, res.Detail = outcome, detail
	return res
}

func (r Recipe) git(step Step, res StepResult, expect string) StepResult {
	dir, err := r.Repo(str(step["repo"]))
	if err != nil {
		return res.with(Unreachable, "repo "+str(step["repo"])+": "+err.Error())
	}
	switch res.Action {
	case "grep":
		pattern := str(step["pattern"])
		if pattern == "" {
			return res.with(Unreachable, "git grep needs a pattern")
		}
		args := []string{"grep", "-q", "-F", "-e", pattern, "HEAD", "--"}
		if p := str(step["path"]); p != "" {
			args = append(args, p)
		}
		code, out, err := r.runGit(dir, args...)
		return judgeGit(res, expect, code, out, err)
	case "exists":
		code, out, err := r.runGit(dir, "ls-tree", "--name-only", "HEAD", "--", str(step["path"]))
		if err == nil && code == 0 && strings.TrimSpace(out) == "" {
			code = 1
		}
		return judgeGit(res, expect, code, out, err)
	case "remote":
		code, out, err := r.runGit(dir, "remote", "get-url", "origin")
		if err != nil || code != 0 {
			return res.with(Unreachable, "git remote get-url origin: "+gitDetail(code, out, err))
		}
		return presence(res, expect, strings.TrimSpace(out) == str(step["url"]), "origin is "+strings.TrimSpace(out))
	}
	return res.with(Unreachable, "unsupported git action "+res.Action+"; use grep, exists or remote")
}

// judgeGit maps a git exit code to a step outcome: 0 found, 1 not found, anything else means
// the repo could not be read.
func judgeGit(res StepResult, expect string, code int, out string, err error) StepResult {
	if err != nil || (code != 0 && code != 1) {
		return res.with(Unreachable, gitDetail(code, out, err))
	}
	return presence(res, expect, code == 0, "")
}

func presence(res StepResult, expect string, found bool, detail string) StepResult {
	if found == (expect == ExpectPresent) {
		return res.with(Match, detail)
	}
	if detail == "" {
		detail = "expected " + expect + ", found the opposite"
	}
	return res.with(Mismatch, detail)
}

func (r Recipe) observed(step Step, res StepResult, expect string, i int) StepResult {
	id, ok := r.Observed[i]
	if !ok {
		return res.with(Unreachable, fmt.Sprintf("run step %d with %s and pass its tool_use_id (--observed %d=<id>)", i, res.Tool, i))
	}
	o, err := r.Observation(id)
	if err != nil {
		return res.with(Unreachable, err.Error())
	}
	if !slices.Contains(r.ToolsOf(o.Tool, []byte(o.Input)), res.Tool) {
		return res.with(Unreachable, "tool call "+id+" did not use "+res.Tool)
	}
	pattern := str(step["pattern"])
	if pattern == "" {
		return res.with(Unreachable, "a step judged from tool output needs a pattern to look for")
	}
	found := strings.Contains(outputText(o.Output), pattern)
	if !found && o.Truncated {
		return res.with(Unreachable, "the captured output of "+id+" was cut before the end")
	}
	return presence(res, expect, found, "")
}

// outputText is the text Claude saw: the string leaves of the JSON output, so quotes and
// newlines are not escaped. Output cut at the size limit is no longer JSON and is used as is.
func outputText(output string) string {
	var v any
	if err := json.Unmarshal([]byte(output), &v); err != nil {
		return output
	}
	return strings.Join(observe.Strings(v), "\n")
}

func (r Recipe) runGit(dir string, args ...string) (int, string, error) {
	if r.Git != nil {
		return r.Git(dir, args...)
	}
	return RunGit(dir, args...)
}

// RunGit runs a read-only git command in dir. System and global git config are ignored, so a
// developer's aliases, hooks or fsmonitor never run, and optional locks are off, so not even the
// index of a developer's clone is touched (FR-DISC-016).
func RunGit(dir string, args ...string) (int, string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "--no-pager"}, args...)...) //nolint:gosec // fixed git subcommands; values are arguments, never shell
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), out.String(), nil
	}
	return 0, out.String(), err
}

func gitDetail(code int, out string, err error) string {
	if err != nil {
		return "git: " + err.Error()
	}
	return fmt.Sprintf("git exit %d: %s", code, strings.TrimSpace(out))
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
