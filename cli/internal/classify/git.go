package classify

import (
	"path"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/shellparse"
)

var gitValueFlags = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--exec-path": true}

var gitRemoteReads = map[string]bool{"fetch": true, "ls-remote": true}

var gitHistoryRewrite = map[string]bool{"filter-branch": true, "filter-repo": true, "replace": true}

var gitReads = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "grep": true, "ls-files": true, "rev-parse": true,
	"blame": true, "describe": true, "shortlog": true, "cat-file": true, "ls-tree": true, "merge-base": true,
	"rev-list": true, "reflog": true, "whatchanged": true, "name-rev": true, "for-each-ref": true, "show-ref": true,
	"var": true, "help": true, "version": true, "count-objects": true, "check-ignore": true, "verify-commit": true,
}

// git classifies git. Pushes are decided in detail; history rewrites are forbidden; other
// writes stay local (W0).
func (cl *cmdClassifier) git(cmd shellparse.Command) Action {
	a := Action{Tool: "git", ToolID: "git", Text: text(cmd)}
	pos, vals := positionals(cmd.Args[1:], gitValueFlags)
	if len(pos) == 0 {
		a.Tier = R0
		return a
	}
	sub := known(pos[0])
	a.Verb = sub
	if sub == "remote" && len(pos) > 1 {
		a.Verb = "remote " + known(pos[1])
	}
	dir := cl.cwd
	if v, ok := vals["-C"]; ok {
		dir = pathOf(cl.cwd, v)
	}
	a.Dir = dir
	switch {
	case gitAliasOnCommandLine(cmd.Args[1:]):
		a.Tier, a.Forbidden = X, "defines a git alias on the command line, which hides the real git command"
	case sub == "push":
		return cl.gitPush(a, cmd.Args[1:], dir)
	case gitHistoryRewrite[sub]:
		a.Tier, a.Forbidden = X, "rewrites git history"
	case gitRemoteReads[sub]:
		a.Tier, a.Remote = R1, true
	case isGitRead(sub, pos, cmd.Args):
		a.Tier = R0
	case sub == "config":
		a.Tier, a.Paths = W0, []string{"$HOME/.gitconfig"}
	case gitLocalWrites[sub]:
		a.Tier, a.Paths = W0, []string{dir}
	default:
		// Not a git command Athel knows: it may be an alias from the repo's or the user's git
		// config (`git p` = `push --force`), so its effect is unknown.
		a.Tier, a.Paths, a.Uncertain = W0, []string{dir}, UnknownCommand
	}
	return a
}

// gitLocalWrites are the git subcommands that change only the local repository.
var gitLocalWrites = map[string]bool{
	"add": true, "commit": true, "checkout": true, "switch": true, "restore": true, "reset": true, "merge": true,
	"rebase": true, "stash": true, "tag": true, "branch": true, "rm": true, "mv": true, "apply": true, "am": true,
	"cherry-pick": true, "revert": true, "clean": true, "init": true, "worktree": true, "pull": true, "notes": true,
	"submodule": true, "gc": true, "prune": true, "update-index": true, "sparse-checkout": true, "bisect": true,
	"format-patch": true, "clone": true, "remote": true, "repack": true, "maintenance": true, "fsck": true,
}

// gitAliasOnCommandLine reports `git -c alias.x=…` (or a -c value Athel cannot read), which
// would let any name stand for any git command.
func gitAliasOnCommandLine(args []shellparse.Word) bool {
	for i, w := range args {
		if !w.Known {
			continue
		}
		v := strings.ToLower(w.Value)
		switch {
		case strings.HasPrefix(v, "-calias."), v == "--config-env" || strings.HasPrefix(v, "--config-env="):
			return true
		case v == "-c" && i+1 < len(args):
			next := args[i+1]
			if !next.Known || strings.HasPrefix(strings.ToLower(next.Value), "alias.") {
				return true
			}
		}
	}
	return false
}

// isGitRead covers read-only subcommands and the read-only forms of remote, config, branch,
// stash and tag.
func isGitRead(sub string, pos, args []shellparse.Word) bool {
	second := ""
	if len(pos) > 1 {
		second = known(pos[1])
	}
	global := hasWord(args, "--global") || hasWord(args, "--system")
	switch sub {
	case "remote":
		return len(pos) == 1 || slices.Contains([]string{"-v", "show", "get-url"}, second)
	case "config":
		return !global && isConfigRead(pos)
	case "branch":
		return isBranchList(args)
	case "stash":
		return second == "list"
	case "tag":
		return len(pos) == 1
	}
	return gitReads[sub]
}

func hasWord(args []shellparse.Word, w string) bool {
	return slices.ContainsFunc(args, func(a shellparse.Word) bool { return a.Known && a.Value == w })
}

func isConfigRead(pos []shellparse.Word) bool {
	return len(pos) <= 2 // `git config key` reads; `git config key value` writes
}

func isBranchList(args []shellparse.Word) bool {
	for _, a := range args[2:] {
		if !a.Known {
			return false
		}
		switch a.Value {
		case "-a", "-r", "-v", "-vv", "--list", "--show-current", "--all", "--remotes", "--merged", "--no-merged", "--contains":
		default:
			return false
		}
	}
	return true
}

var gitPushValueFlags = map[string]bool{"--repo": true, "-o": true, "--push-option": true, "--receive-pack": true, "--exec": true}

// gitPush: force, mirror, delete and pushes to protected branches are forbidden (FR-GIT-004);
// anything else is W1 on the branches it targets.
func (cl *cmdClassifier) gitPush(a Action, args []shellparse.Word, dir string) Action {
	a.Remote = true
	pos, vals := positionals(args[1:], gitPushValueFlags)
	for _, f := range []string{"-f", "--force", "--force-with-lease", "--force-if-includes", "--mirror", "-d", "--delete", "--prune"} {
		if _, ok := vals[f]; ok {
			a.Tier, a.Forbidden = X, "git push "+f+" (force push, mirror or branch deletion)"
			return a
		}
	}
	if _, ok := vals["--all"]; ok {
		a.Tier, a.Forbidden = X, "git push --all pushes protected branches"
		return a
	}
	refspecs := pos
	if len(pos) > 0 {
		refspecs = pos[1:] // the first positional is the remote
	}
	if len(refspecs) == 0 {
		branch := cl.currentBranch(dir)
		refspecs = []shellparse.Word{{Value: branch, Known: branch != "?"}}
	}
	for _, r := range refspecs {
		dst, forbidden := pushTarget(r)
		if forbidden != "" {
			a.Tier, a.Forbidden = X, forbidden
			return a
		}
		if dst != "?" && cl.isProtected(dst) {
			a.Tier, a.Forbidden = X, "pushes to protected branch "+dst
			return a
		}
		a.Branches = append(a.Branches, dst)
	}
	a.Tier = W1
	return a
}

// pushTarget returns the remote branch a refspec writes, or why the refspec is forbidden.
func pushTarget(r shellparse.Word) (string, string) {
	if !r.Known {
		return "?", ""
	}
	spec := r.Value
	if strings.HasPrefix(spec, "+") {
		return "", "force push refspec " + spec
	}
	src, dst, hasDst := strings.Cut(spec, ":")
	if hasDst && src == "" {
		return "", "deletes remote branch " + dst
	}
	if !hasDst {
		dst = src
	}
	dst = strings.TrimPrefix(dst, "refs/heads/")
	if dst == "HEAD" || dst == "" {
		return "?", ""
	}
	return dst, ""
}

func (cl *cmdClassifier) isProtected(branch string) bool {
	if cl.pol == nil {
		return true
	}
	if branch == cl.pol.Git.BaseBranch {
		return true
	}
	for _, p := range cl.pol.Git.Protected {
		if ok, _ := path.Match(p, branch); ok || p == branch {
			return true
		}
	}
	return false
}

// currentBranch reads the checked-out branch from .git/HEAD (following a worktree's gitdir
// file); "?" when it cannot be known.
func (cl *cmdClassifier) currentBranch(dir string) string {
	if cl.read == nil {
		return "?"
	}
	gitPath := path.Join(dir, ".git")
	head, err := cl.read(path.Join(gitPath, "HEAD"))
	if err != nil {
		link, lerr := cl.read(gitPath)
		gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(link)), "gitdir: ")
		if lerr != nil || !ok {
			return "?"
		}
		if !path.IsAbs(gitdir) {
			gitdir = path.Join(dir, gitdir)
		}
		if head, err = cl.read(path.Join(gitdir, "HEAD")); err != nil {
			return "?"
		}
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: refs/heads/")
	if !ok {
		return "?" // detached HEAD
	}
	return ref
}

var ghValueFlags = map[string]bool{"-R": true, "--repo": true, "-X": true, "--method": true, "-f": true, "-F": true,
	"--field": true, "--raw-field": true, "-H": true, "--header": true, "--input": true, "-b": true, "--body": true,
	"-t": true, "--title": true, "-B": true, "--base": true, "--head": true, "-q": true, "--jq": true, "-L": true, "--limit": true}

// ghRule decides one family of gh commands.
type ghRule struct {
	match     func(group, sub string) bool
	tier      Tier
	forbidden string
}

func oneOf(list ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(list, v) }
}

func both(g, s func(string) bool) func(string, string) bool {
	return func(group, sub string) bool { return g(group) && s(sub) }
}

func anySub(string) bool { return true }

// ghRules are checked in order: merging is forbidden; anything that can trigger a pipeline or
// a release is W3; comments on issues are shared writes; own PR edits are W1.
var ghRules = []ghRule{
	{match: both(oneOf("pr"), oneOf("merge")), tier: X, forbidden: "merges a pull request; humans merge"},
	{match: both(oneOf("auth"), oneOf("token")), tier: X, forbidden: "prints a GitHub token"},
	{match: both(oneOf("pr"), oneOf("create", "edit", "ready", "close", "reopen", "comment", "review")), tier: W1},
	{match: both(oneOf("issue"), oneOf("create", "comment", "edit", "close", "reopen", "delete", "transfer", "lock")), tier: W2},
	{match: both(oneOf("run", "workflow"), oneOf("rerun", "run", "enable", "disable", "cancel", "delete")), tier: W3},
	{match: both(oneOf("release"), func(s string) bool { return !slices.Contains([]string{"list", "view", "download"}, s) }), tier: W3},
	{match: both(oneOf("repo"), oneOf("create", "delete", "edit", "archive", "rename", "fork", "sync")), tier: W3},
	{match: both(oneOf("secret", "variable", "ruleset", "cache"), func(s string) bool { return s != "list" }), tier: W3},
	{match: both(oneOf("auth", "config", "extension", "alias"), anySub), tier: W0},
}

// gh classifies the GitHub CLI.
func (cl *cmdClassifier) gh(cmd shellparse.Command) Action {
	a := Action{Tool: "gh", ToolID: "gh", Text: text(cmd), Remote: true, Tier: R1}
	pos, vals := positionals(cmd.Args[1:], ghValueFlags)
	if len(pos) == 0 {
		a.Tier = R0
		return a
	}
	group, sub := known(pos[0]), ""
	if len(pos) > 1 {
		sub = known(pos[1])
	}
	a.Verb = strings.TrimSpace(group + " " + sub)
	if group == "api" {
		a.Tier, a.Forbidden = ghAPI(pos, vals)
		return a
	}
	for _, r := range ghRules {
		if r.match(group, sub) {
			a.Tier, a.Forbidden = r.tier, r.forbidden
			break
		}
	}
	return a
}

func ghAPI(pos []shellparse.Word, vals map[string]shellparse.Word) (Tier, string) {
	method := strings.ToUpper(flagVal(vals, "-X") + flagVal(vals, "--method"))
	endpoint := ""
	if len(pos) > 1 {
		endpoint = known(pos[1])
	}
	_, hasField := vals["-f"]
	_, hasField2 := vals["-F"]
	_, hasInput := vals["--input"]
	if method == "" && (hasField || hasField2 || hasInput) {
		method = "POST"
	}
	switch {
	case strings.Contains(endpoint, "/merge") && method != "" && method != "GET":
		return X, "merges through the GitHub API; humans merge"
	case method == "" || method == "GET" || method == "HEAD":
		return R1, ""
	case method == "DELETE" || strings.Contains(endpoint, "/dispatches") || strings.Contains(endpoint, "/deployments"):
		return W3, ""
	}
	return W2, ""
}
