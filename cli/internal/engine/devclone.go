package engine

import (
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// devCloneGitReads are the git commands doc 04's rule 12 exception allows in a developer's
// clone. Any other git command there is denied: Athel works in its own cache clone (FR-DISC-016).
var devCloneGitReads = map[string]bool{
	"remote get-url": true, "log": true, "show": true, "grep": true, "ls-files": true, "status": true, "rev-parse": true,
}

// devCloneGit decides a git command whose repository is a developer's clone.
func (ctx Context) devCloneGit(a classify.Action, verdict verdictFn) ActionDecision {
	if devCloneGitReads[a.Verb] {
		return verdict(Allow, "12", "", "read-only git in the developer's clone "+a.Dir+" (doc 04 rule 12 exception)")
	}
	return verdict(Deny, "12", aerr.PolicyDevClone, "git "+a.Verb+" in the developer's clone "+a.Dir+
		" could change it; Athel reads developer clones only and works in its own cache clone (FR-DISC-016)")
}

// devClone reports whether p is inside a developer's clone: under a confirmed search root, in
// a folder that has a .git folder, and not inside Athel's own workspace (its worktrees and cache
// clones live there).
func (ctx Context) devClone(p string) bool {
	if rest, ok := strings.CutPrefix(p, "$HOME"); ok && ctx.Resolver != nil && ctx.Resolver.Home != "" {
		p = ctx.Resolver.Home + rest
	}
	if !path.IsAbs(p) || ctx.inWorkspace(p) {
		return false
	}
	p = path.Clean(p)
	for _, root := range ctx.SearchRoots {
		if !within(p, root) {
			continue
		}
		for dir := p; within(dir, root); dir = path.Dir(dir) {
			if info, err := ctx.stat(path.Join(dir, ".git")); err == nil && info.IsDir() {
				return true
			}
			if dir == root {
				break
			}
		}
	}
	return false
}

func (ctx Context) stat(p string) (fs.FileInfo, error) {
	if ctx.Stat != nil {
		return ctx.Stat(p)
	}
	return os.Lstat(p)
}

func within(p, root string) bool { return p == root || strings.HasPrefix(p, root+"/") }
