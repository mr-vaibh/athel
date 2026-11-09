package engine

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

type dirInfo struct{ dir bool }

func (d dirInfo) Name() string       { return ".git" }
func (d dirInfo) Size() int64        { return 0 }
func (d dirInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (d dirInfo) ModTime() time.Time { return time.Time{} }
func (d dirInfo) IsDir() bool        { return d.dir }
func (d dirInfo) Sys() any           { return nil }

// withRoots confirms /home/dev/Developer, holding the developer's clone "users" and a worktree
// "wt" whose .git is a file.
func withRoots(c *Context) {
	c.SearchRoots = []string{"/home/dev/Developer"}
	c.Stat = func(p string) (fs.FileInfo, error) {
		switch p {
		case "/home/dev/Developer/users/.git":
			return dirInfo{dir: true}, nil
		case "/home/dev/Developer/wt/.git":
			return dirInfo{dir: false}, nil
		}
		return nil, errors.New("no such file")
	}
}

func TestDevCloneReadOnlyGit(t *testing.T) {
	git := func(cmd string) map[string]any { _, in := bash(cmd); return in }
	in := "git -C /home/dev/Developer/users "
	run(t, []ruleCase{
		{"log", "Bash", git(in + "log -5"), withRoots, Allow, "12", ""},
		{"remote get-url", "Bash", git(in + "remote get-url origin"), withRoots, Allow, "12", ""},
		{"status", "Bash", git(in + "status --short"), withRoots, Allow, "12", ""},
		{"grep in a subfolder", "Bash", git("git -C /home/dev/Developer/users/src grep -n phone"), withRoots, Allow, "12", ""},
		{"fetch", "Bash", git(in + "fetch origin"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"checkout", "Bash", git(in + "checkout -b x"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"worktree add", "Bash", git(in + "worktree add ../x"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"stash", "Bash", git(in + "stash"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"remote add", "Bash", git(in + "remote add x y"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"unknown alias", "Bash", git(in + "sync"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"$HOME form", "Bash", git("git -C ~/Developer/users fetch"), withRoots, Deny, "12", aerr.PolicyDevClone},
		{"outside the roots", "Bash", git("git -C /home/dev/other/repo fetch"), withRoots, Allow, "11", ""},
		{"a worktree is not a clone", "Bash", git("git -C /home/dev/Developer/wt fetch"), withRoots, Allow, "11", ""},
		{"no roots confirmed", "Bash", git(in + "fetch"), nil, Allow, "11", ""},
		{"guard mode leaves the developer's own git alone", "Bash", git(in + "commit -m x"), func(c *Context) { withRoots(c); c.Mode = Guard }, None, "-", ""},
	})
}

func TestReadsUnderRootsAllowed(t *testing.T) {
	read := func(p string) map[string]any { return map[string]any{"file_path": p} }
	run(t, []ruleCase{
		{"file in a developer clone", "Read", read("/home/dev/Developer/users/src/user.go"), withRoots, Allow, "12", ""},
		{"file outside any clone", "Read", read("/home/dev/Developer/notes.txt"), withRoots, Approve, "12", ""},
		{"file outside the roots", "Read", read("/home/dev/Documents/x.txt"), withRoots, Approve, "12", ""},
		{"secret in a developer clone", "Read", read("/home/dev/Developer/users/.env"), withRoots, Deny, "12", aerr.PolicySecretPath},
	})
}

func TestScoutWritesDenied(t *testing.T) {
	scout := func(c *Context) { c.Agent, c.Subagent = hookio.ScoutAgent, true }
	scoutGuard := func(c *Context) { scout(c); c.Mode = Guard }
	_, touch := bash("touch " + tree + "/x")
	_, factAdd := bash("athel fact add -")
	_, kubectl := bash("kubectl --context kind-dev get pods")
	edit := map[string]any{"file_path": tree + "/src/a.go", "old_string": "a", "new_string": "b"}
	run(t, []ruleCase{
		{"edit", "Edit", edit, scout, Deny, "3", aerr.PolicyAgentReadOnly},
		{"shell write", "Bash", touch, scout, Deny, "3", aerr.PolicyAgentReadOnly},
		{"shell write in guard mode", "Bash", touch, scoutGuard, Deny, "3", aerr.PolicyAgentReadOnly},
		{"its own athel CLI calls", "Bash", factAdd, scout, Allow, "7x", ""},
		{"reads", "Bash", kubectl, scout, Allow, "11", ""},
		{"another subagent may write in the plan", "Edit", edit, func(c *Context) { c.Agent, c.Subagent = "athel:developer", true }, Allow, "10", ""},
	})
}

func TestKnowledgeReadDenied(t *testing.T) {
	_, cat := bash("cat " + ws + "/.athel/knowledge/facts.jsonl")
	_, find := bash("athel fact find --subject users")
	run(t, []ruleCase{
		{"Read tool", "Read", map[string]any{"file_path": ws + "/.athel/knowledge/facts.jsonl"}, nil, Deny, "3", aerr.PolicyForbidden},
		{"Grep tool", "Grep", map[string]any{"pattern": "x", "path": ws + "/.athel/knowledge"}, nil, Deny, "3", aerr.PolicyForbidden},
		{"cat", "Bash", cat, nil, Deny, "3", aerr.PolicyForbidden},
		{"the CLI", "Bash", find, nil, Allow, "7x", ""},
	})
	if (Context{}).devClone("relative/path") || !within("/a/b", "/a") || within("/ab", "/a") {
		t.Error("path helpers")
	}
	if _, err := (Context{}).stat("/definitely/not/here"); err == nil {
		t.Error("default stat")
	}
}
