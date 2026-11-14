package engine

import (
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

func as(agent string) func(*Context) {
	return func(c *Context) { c.Agent, c.Subagent = agent, true }
}

func edit(p string) map[string]any {
	return map[string]any{"file_path": p, "old_string": "a", "new_string": "b"}
}

// TestQAWritesOnlyTests (ADR 024): QA writes test files and runs tests; a product fix is the
// developer's.
func TestQAWritesOnlyTests(t *testing.T) {
	qa := as(hookio.QAAgent)
	_, goTest := bash("go test ./...")
	_, npmTest := bash("npm test")
	_, commit := bash("git commit -m x")
	_, sed := bash("sed -i s/a/b/ " + tree + "/src/a.go")
	_, touch := bash("touch " + tree + "/src/a_test.go")
	_, save := bash("athel qa save CHG-001 T1 -")
	_, review := bash("athel review save CHG-001 T1 -")
	_, read := bash("athel review show CHG-001 T1")
	_, push := bash("git push origin athel/CHG-001-svc")
	run(t, []ruleCase{
		{"a Go test file", "Edit", edit(tree + "/src/a_test.go"), qa, Allow, "10", ""},
		{"a test folder", "Write", map[string]any{"file_path": tree + "/src/tests/b.py", "content": "x"}, qa, Allow, "10", ""},
		{"a product file", "Edit", edit(tree + "/src/a.go"), qa, Deny, "3", aerr.PolicyAgentScope},
		{"a test file outside the worktrees", "Edit", edit("/ws/tests/a_test.go"), qa, Deny, "3", aerr.PolicyAgentScope},
		{"a product file through the shell", "Bash", sed, qa, Deny, "3", aerr.PolicyAgentScope},
		{"a test file through the shell", "Bash", touch, qa, Allow, "10", ""},
		{"go test", "Bash", goTest, qa, Allow, "10", ""},
		{"npm test", "Bash", npmTest, qa, Allow, "10", ""},
		{"commit", "Bash", commit, qa, Deny, "3", aerr.PolicyAgentScope},
		{"push", "Bash", push, qa, Deny, "3", aerr.PolicyAgentScope},
		{"its report", "Bash", save, qa, Allow, "7x", ""},
		{"the reviewer's round", "Bash", review, qa, Deny, "3", aerr.PolicyAgentScope},
		{"a read", "Bash", read, qa, Allow, "7x", ""},
	})
}

// TestReviewerCannotWrite (ADR 024): the checkers judge and never change what they judge.
func TestReviewerCannotWrite(t *testing.T) {
	_, touch := bash("touch " + tree + "/src/a_test.go")
	_, goTest := bash("go test ./...")
	_, fix := bash("athel review fix CHG-001 T1 R1-01 --commit HEAD")
	_, save := bash("athel review save CHG-001 T1 -")
	_, sec := bash("athel security save CHG-001 T1 -")
	_, finding := bash("athel finding add CHG-001 -")
	_, gate := bash("athel gate show CHG-001 T1")
	_, waive := bash("athel gate waive CHG-001 T1 R1-01 --until 2026-12-01 --reason ok")
	for _, agent := range []string{hookio.ReviewerAgent, hookio.SecurityAgent} {
		t.Run(agent, func(t *testing.T) {
			run(t, []ruleCase{
				{"edit", "Edit", edit(tree + "/src/a.go"), as(agent), Deny, "3", aerr.PolicyAgentReadOnly},
				{"shell write", "Bash", touch, as(agent), Deny, "3", aerr.PolicyAgentReadOnly},
				{"a command in the worktree", "Bash", goTest, as(agent), Deny, "3", aerr.PolicyAgentReadOnly},
				{"the developer's fix", "Bash", fix, as(agent), Deny, "3", aerr.PolicyAgentScope},
				{"a side finding", "Bash", finding, as(agent), Allow, "7x", ""},
				{"gate results", "Bash", gate, as(agent), Allow, "7x", ""},
				{"a waiver stays the human's", "Bash", waive, as(agent), Deny, "H", aerr.PolicyHumanOrigin},
			})
		})
	}
	run(t, []ruleCase{
		{"the reviewer saves its round", "Bash", save, as(hookio.ReviewerAgent), Allow, "7x", ""},
		{"security cannot save the round", "Bash", save, as(hookio.SecurityAgent), Deny, "3", aerr.PolicyAgentScope},
		{"security saves its report", "Bash", sec, as(hookio.SecurityAgent), Allow, "7x", ""},
	})
}

// TestDeveloperCannotGradeItself: the developer writes its task's code but never saves a
// review round, runs gates or ships (ADR 024).
func TestDeveloperCannotGradeItself(t *testing.T) {
	dev := as(hookio.DeveloperAgent)
	_, review := bash("athel review save CHG-001 T1 -")
	_, gate := bash("athel gate run --stage full CHG-001 T1")
	_, done := bash("athel build done CHG-001 T1")
	_, fix := bash("athel review fix CHG-001 T1 R1-01 --commit HEAD")
	_, commit := bash("git commit -m x")
	run(t, []ruleCase{
		{"its task's code", "Edit", edit(tree + "/src/a.go"), dev, Allow, "10", ""},
		{"outside the plan", "Edit", edit(tree + "/main.go"), dev, Deny, "7", aerr.PolicyScope},
		{"commit", "Bash", commit, dev, Allow, "10", ""},
		{"marks a fix", "Bash", fix, dev, Allow, "7x", ""},
		{"saves a review round", "Bash", review, dev, Deny, "3", aerr.PolicyAgentScope},
		{"runs the gates", "Bash", gate, dev, Deny, "3", aerr.PolicyAgentScope},
		{"marks the task done", "Bash", done, dev, Deny, "3", aerr.PolicyAgentScope},
		{"the main session may", "Bash", done, nil, Allow, "7x", ""},
	})
}
