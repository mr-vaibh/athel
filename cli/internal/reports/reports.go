// Package reports is doc 06's role-agent outputs besides the review round: the developer's
// build-result@1, QA's qa-report@1 and the security agent's security-report@1. The CLI fills in
// what it can know itself (the changed files, IDs, the commit); the agent gives the rest.
package reports

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/quality"
)

// Schema IDs.
const (
	BuildSchema    = "athel/build-result@1"
	QASchema       = "athel/qa-report@1"
	SecuritySchema = "athel/security-report@1"
)

// Results of a command or a test case.
const (
	Pass = "pass"
	Fail = "fail"
)

var (
	// QA's job is negative, boundary and failure tests (doc 06 role table).
	categories = []string{"negative", "boundary", "failure"}
	// The review scale, so report findings join review rounds unchanged.
	severities = []string{"blocker", "major", "minor", "nit"}
	cweRe      = regexp.MustCompile(`^CWE-[1-9][0-9]*$`)
)

// Command is a command an agent ran and how it ended.
type Command struct {
	Command string `yaml:"command" json:"command"`
	Result  string `yaml:"result" json:"result"`
}

// Finding is a problem QA or security found in the task's code; it becomes a finding of the
// task's review round (source qa or security) and is fixed and verified like any other.
type Finding struct {
	ID       string `yaml:"id" json:"id"`
	Severity string `yaml:"severity" json:"severity"`
	CWE      string `yaml:"cwe,omitempty" json:"cwe,omitempty"`
	Location string `yaml:"location" json:"location"`
	Rule     string `yaml:"rule" json:"rule"`
	Finding  string `yaml:"finding" json:"finding"`
	Required string `yaml:"required" json:"required"`
}

// BuildDraft is what the developer sends; Build is what the CLI saves.
type BuildDraft struct {
	CommandsRun []Command `yaml:"commands_run"`
	Notes       string    `yaml:"notes"`
}

// Build is build-result@1 (build/<task>/result-N.yaml).
type Build struct {
	Schema       string    `yaml:"schema" json:"schema"`
	Change       string    `yaml:"change" json:"change"`
	Task         string    `yaml:"task" json:"task"`
	N            int       `yaml:"n" json:"n"`
	Head         string    `yaml:"head" json:"head"`
	FilesChanged []string  `yaml:"files_changed" json:"files_changed"`
	TestsAdded   []string  `yaml:"tests_added" json:"tests_added"`
	CommandsRun  []Command `yaml:"commands_run" json:"commands_run"`
	Notes        string    `yaml:"notes" json:"notes"`
}

// Case is one test QA added.
type Case struct {
	Category string `yaml:"category" json:"category"`
	File     string `yaml:"file" json:"file"`
	Name     string `yaml:"name" json:"name"`
	Covers   string `yaml:"covers,omitempty" json:"covers,omitempty"`
	Result   string `yaml:"result" json:"result"`
}

// QADraft is what QA sends; QA is what the CLI saves.
type QADraft struct {
	Cases    []Case    `yaml:"cases"`
	Suite    Command   `yaml:"suite"`
	Gaps     []string  `yaml:"gaps"`
	Findings []Finding `yaml:"findings"`
}

// QA is qa-report@1 (reviews/<task>/qa-N.yaml).
type QA struct {
	Schema   string    `yaml:"schema" json:"schema"`
	Change   string    `yaml:"change" json:"change"`
	Task     string    `yaml:"task" json:"task"`
	N        int       `yaml:"n" json:"n"`
	Head     string    `yaml:"head" json:"head"`
	Cases    []Case    `yaml:"cases" json:"cases"`
	Suite    Command   `yaml:"suite" json:"suite"`
	Gaps     []string  `yaml:"gaps" json:"gaps"`
	Findings []Finding `yaml:"findings" json:"findings"`
}

// SecurityDraft is what the security agent sends; Security is what the CLI saves.
type SecurityDraft struct {
	Checked  []string  `yaml:"checked"`
	Findings []Finding `yaml:"findings"`
}

// Security is security-report@1 (reviews/<task>/security-N.yaml).
type Security struct {
	Schema   string    `yaml:"schema" json:"schema"`
	Change   string    `yaml:"change" json:"change"`
	Task     string    `yaml:"task" json:"task"`
	N        int       `yaml:"n" json:"n"`
	Head     string    `yaml:"head" json:"head"`
	Checked  []string  `yaml:"checked" json:"checked"`
	Findings []Finding `yaml:"findings" json:"findings"`
}

func invalid(kind, msg string) error {
	return aerr.New(aerr.StateInvalid, kind+" rejected: "+msg).
		WithNextAction("fix it and save it again; athel " + kind + " template shows the format")
}

// CheckBuild checks a developer's draft.
func CheckBuild(d BuildDraft) error {
	if len(d.CommandsRun) == 0 {
		return invalid("build", "list the build, test and lint commands you ran (commands_run)")
	}
	return checkCommands("build", d.CommandsRun...)
}

func checkCommands(kind string, cs ...Command) error {
	for _, c := range cs {
		if strings.TrimSpace(c.Command) == "" || c.Result != Pass && c.Result != Fail {
			return invalid(kind, "every command has its text and a result, pass or fail")
		}
	}
	return nil
}

// CheckQA checks QA's draft against the test files the task changed: every case is a test QA
// wrote, and a failing case is a finding for the developer, never a fix (doc 06).
func CheckQA(d QADraft, isTest func(string) bool, changed []string) error {
	if len(d.Cases) == 0 {
		return invalid("qa", "a QA report lists the cases added")
	}
	failing := false
	for _, c := range d.Cases {
		switch {
		case !slices.Contains(categories, c.Category):
			return invalid("qa", "a case's category is one of "+strings.Join(categories, ", "))
		case c.Name == "" || c.Result != Pass && c.Result != Fail:
			return invalid("qa", "every case has its test name and a result, pass or fail")
		case !isTest(c.File):
			return invalid("qa", c.File+" is not a test file")
		case !slices.Contains(changed, c.File):
			return invalid("qa", c.File+" is not changed by this task")
		}
		failing = failing || c.Result == Fail
	}
	if err := checkCommands("qa", d.Suite); err != nil {
		return err
	}
	if (failing || d.Suite.Result == Fail) && len(d.Findings) == 0 {
		return invalid("qa", "a failing test shows a bug: file it under findings for the developer")
	}
	return checkFindings("qa", d.Findings, false)
}

// CheckSecurity checks the security agent's draft.
func CheckSecurity(d SecurityDraft) error {
	if len(d.Checked) == 0 {
		return invalid("security", "list what you checked (checked)")
	}
	return checkFindings("security", d.Findings, true)
}

func checkFindings(kind string, fs []Finding, cwe bool) error {
	for _, f := range fs {
		switch {
		case !slices.Contains(severities, f.Severity):
			return invalid(kind, "severity is one of "+strings.Join(severities, ", "))
		case cwe && !cweRe.MatchString(f.CWE):
			return invalid(kind, "every security finding names its CWE category, like CWE-89")
		case f.Location == "" || f.Rule == "" || f.Finding == "" || f.Required == "":
			return invalid(kind, "every finding has a location, a rule, the finding and the required fix")
		case !quality.Known(f.Rule):
			return invalid(kind, f.Rule+" is not a rule of the code quality standard; athel review rules lists them")
		}
	}
	return nil
}

// Number gives report n's findings their IDs: Q<n>-NN for QA, S<n>-NN for security.
func Number(prefix string, n int, fs []Finding) []Finding {
	out := make([]Finding, len(fs))
	for i, f := range fs {
		f.ID = fmt.Sprintf("%s%d-%02d", prefix, n, i+1)
		out[i] = f
	}
	return out
}
