// Package engine is Athel's policy decision (doc 04 §Permission tiers and the policy decision):
// a fixed, ordered rule list applied to every action of a tool call; the first matching rule
// decides an action and the strictest action decides the call.
//
// Two modes (decision Q1, ADR 015). Guard mode applies in every session: rules 0–6 (invalid
// policy, malformed calls, unparseable writes, forbidden actions, prod writes, irreversible
// actions). Change mode applies while the session holds an Athel change lock: all 13 rules,
// including the plan scope (7), own branches (9) and the worktree (10). Outside a change,
// anything the guard rules do not stop gets no decision from Athel and follows Claude Code's
// normal permission flow.
package engine

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/envdetect"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/policy"
)

// Decision values. None means Athel leaves the call to Claude Code's normal permission flow.
type Decision string

// Decisions, from weakest to strictest.
const (
	None    Decision = "none"
	Allow   Decision = "allow"
	Approve Decision = "approve"
	Deny    Decision = "deny"
)

var strength = map[Decision]int{None: 0, Allow: 1, Approve: 2, Deny: 3}

// Mode is guard or change.
type Mode string

// Modes.
const (
	Guard  Mode = "guard"
	Change Mode = "change"
)

// Context is everything the rules need besides the call.
type Context struct {
	Mode          Mode
	ChangeID      string
	Plan          *plan.Plan // the approved plan of the active change, nil if none
	WorkspaceRoot string     // directory that holds .athel/ (and repos/ for worktrees)
	OwnBranches   []string   // branches Athel created for this change (from M4)
	Policy        *policy.Set
	PolicyErr     error
	Headless      bool
	Agent         string // subagent type; "" for the main agent
	Subagent      bool   // the call carries an agent_id
	Resolver      *envdetect.Resolver
	// HumanGrant reports whether the human granted change → target in this session (grants).
	HumanGrant func(change, target string) bool
	// SearchRoots are the confirmed repo search roots, absolute (doc 04 rule 12 exception); Stat
	// looks for .git folders below them (os.Lstat when nil).
	SearchRoots []string
	Stat        func(string) (fs.FileInfo, error)
}

// ActionDecision is the verdict on one action.
type ActionDecision struct {
	Action   classify.Action `json:"action"`
	Decision Decision        `json:"decision"`
	Rule     string          `json:"rule"`
	Code     aerr.Code       `json:"code,omitempty"`
	Why      string          `json:"why"`
	Env      string          `json:"env"`
	EnvHow   string          `json:"env_how,omitempty"`
	Task     string          `json:"task,omitempty"`
}

// Result is the decision for a whole call.
type Result struct {
	Decision Decision         `json:"decision"`
	Rule     string           `json:"rule"`
	Code     aerr.Code        `json:"code,omitempty"`
	Tier     classify.Tier    `json:"tier"`
	Env      string           `json:"env"`
	Reason   string           `json:"reason"`
	Mode     Mode             `json:"mode"`
	Actions  []ActionDecision `json:"actions"`
}

// Decide applies the rules to a classified call.
func Decide(call classify.Call, cr classify.Result, ctx Context) Result {
	res := Result{Mode: ctx.Mode, Decision: None, Rule: "-", Tier: classify.R0, Env: envdetect.NotApplicable}
	if cr.Malformed != "" { // rule 1
		res.Decision, res.Rule, res.Code = Deny, "1", aerr.PolicyMalformed
		res.Reason = denyText("malformed tool call: "+cr.Malformed, res.Code, "1")
		return res
	}
	actions := cr.Actions
	if len(actions) == 0 && cr.Unparseable {
		actions = []classify.Action{{Tool: call.Tool, Tier: classify.W0, Text: call.Tool, Uncertain: classify.UnknownCommand}}
	}
	for _, a := range actions {
		d := ctx.decideAction(a, call.Cwd, cr.Unparseable, strings.Join(cr.Reasons, "; "))
		res.Actions = append(res.Actions, d)
		if classify.Rank(a.Tier) > classify.Rank(res.Tier) {
			res.Tier = a.Tier
		}
		if stricter(d, res) {
			res.Decision, res.Rule, res.Code, res.Env = d.Decision, d.Rule, d.Code, d.Env
		}
	}
	res.Reason = ctx.reason(res)
	if ctx.Headless && res.Decision == Approve {
		res.Decision, res.Code = Deny, aerr.PolicyHeadless
		res.Rule += "+headless"
		res.Reason = denyText("needs a human, and this run is headless; "+res.Reason, res.Code, res.Rule)
	}
	return res
}

// stricter reports whether d should decide the call instead of the current result: a stronger
// decision wins; between equal decisions the stricter environment is reported (doc 04: a command
// that targets several environments is decided as the stricter one).
func stricter(d ActionDecision, res Result) bool {
	switch {
	case strength[d.Decision] != strength[res.Decision]:
		return strength[d.Decision] > strength[res.Decision]
	case res.Rule == "-":
		return true
	}
	return envdetect.Strictness(d.Env) > envdetect.Strictness(res.Env)
}

func (ctx Context) decideAction(a classify.Action, cwd string, unparseable bool, why string) ActionDecision {
	d := &ActionDecision{Action: a, Env: envdetect.NotApplicable}
	verdict := func(dec Decision, rule string, code aerr.Code, text string) ActionDecision {
		d.Decision, d.Rule, d.Code, d.Why = dec, rule, code, text
		return *d
	}
	write := classify.IsWrite(a.Tier)
	if ctx.PolicyErr != nil && write { // rule 0, FR-SAFE-012
		return verdict(Deny, "0", aerr.PolicyConfigInvalid, "the policy files are invalid, so every write is disabled: "+ctx.PolicyErr.Error())
	}
	if unparseable && (write || a.Uncertain != "") { // rule 2, FR-SAFE-010
		return verdict(Deny, "2", aerr.PolicyUnparseable, "the command cannot be analysed with confidence ("+why+")")
	}
	if rule, code, text := ctx.forbidden(a); rule != "" {
		return verdict(Deny, rule, code, text)
	}
	if ctx.Resolver != nil { // rule 4
		r := ctx.Resolver.Resolve(a)
		d.Env, d.EnvHow = r.Env, r.How
	}
	if write && d.Env == "prod" { // rule 5, FR-SAFE-002/003
		return verdict(Deny, "5", aerr.PolicyProdWrite, "writes to prod ("+d.EnvHow+")")
	}
	if a.Tier == classify.W3 { // rule 6
		return verdict(Deny, "6", aerr.PolicyIrreversible, "irreversible or broad action (deploys, infrastructure, deletes, database writes and Raptor are always the developer's)")
	}
	if unparseable { // rule 2 for reads
		return verdict(Approve, "2", "", "the command cannot be analysed with confidence ("+why+")")
	}
	if ctx.Mode != Change {
		return verdict(None, "-", "", "no Athel change is active in this session; Claude Code's own permission rules apply")
	}
	return ctx.changeRules(d, a, cwd, verdict)
}

// forbidden applies rule 3 (tier X; secret files are rule 12's always-deny list) and the
// human-origin rule H (doc 02 §Transition rules; M1 plan §4.5). rule is "" when neither denies.
func (ctx Context) forbidden(a classify.Action) (string, aerr.Code, string) {
	switch {
	case a.Tier == classify.X && strings.Contains(a.Forbidden, "a secret file"):
		return "12", aerr.PolicySecretPath, a.Forbidden
	case a.Tier == classify.X:
		return "3", aerr.PolicyForbidden, a.Forbidden
	case ctx.Agent == hookio.ScoutAgent && classify.IsWrite(a.Tier) && a.Athel == nil:
		return "3", aerr.PolicyAgentReadOnly, "the scout is read-only: it reports, it never changes anything (doc 04 injection-defence layer 4)"
	case a.Athel != nil && a.Athel.Command == "integrity accept":
		return "H", aerr.PolicyHumanOrigin, "only the developer accepts a hand edit of Athel state, in a terminal"
	case a.Athel != nil && a.Athel.ByHuman:
		if why := ctx.humanOrigin(a.Athel); why != "" {
			return "H", aerr.PolicyHumanOrigin, why
		}
	}
	return "", "", ""
}

// humanOrigin explains why a human-only CLI call (`--by human`, `repos roots set`) is not the
// human's, or returns "".
func (ctx Context) humanOrigin(call *classify.AthelCall) string {
	switch {
	case ctx.Subagent:
		return "a subagent cannot act as the human (" + ctx.Agent + ")"
	case ctx.HumanGrant != nil && ctx.HumanGrant(call.Change, call.To):
		return ""
	case call.To == grants.TargetDecide:
		return "answering decision cards needs the developer's own prompt /athel:decide CHG-NNN D-1=… with exactly these answers in this session, used once within 10 minutes"
	case call.To == grants.TargetRoots:
		return "saving repo search roots needs the developer's own prompt /athel:roots with exactly these folders in this session, used once within 10 minutes"
	default:
		return fmt.Sprintf("--by human for %s → %s needs the developer's own prompt /athel:approve or /athel:accept %s in this session, used once within 10 minutes",
			call.Change, call.To, call.Change)
	}
}

type verdictFn func(Decision, string, aerr.Code, string) ActionDecision

// changeRules are rules 7–13, applied while the session works on a change.
func (ctx Context) changeRules(d *ActionDecision, a classify.Action, cwd string, verdict verdictFn) ActionDecision {
	if a.Athel != nil { // rule 7 exception: .athel/changes/<id>/ is written only through the CLI
		return verdict(Allow, "7x", "", "athel CLI call")
	}
	if a.Tool == "git" && ctx.devClone(a.Dir) {
		return ctx.devCloneGit(a, verdict)
	}
	if a.Uncertain != "" {
		return verdict(Approve, "7u", "", "Athel does not know what this command does ("+a.Uncertain+")")
	}
	if classify.IsWrite(a.Tier) {
		task, why := ctx.planTask(a, cwd)
		if task == nil { // rule 7, FR-SAFE-004
			return verdict(Deny, "7", aerr.PolicyScope, why)
		}
		d.Task = task.ID
	}
	switch a.Tier {
	case classify.W2: // rule 8
		return ctx.toolPolicy(d, a, "write", verdict, "8")
	case classify.W1:
		return ctx.ownBranches(a, verdict)
	case classify.W0:
		return ctx.worktreeWrite(a, verdict)
	case classify.R1: // rule 11
		if a.ToolID == "" {
			return verdict(Approve, "11", "", "remote read with a tool that is not registered in tools.yaml")
		}
		return ctx.toolPolicy(d, a, "read", verdict, "11")
	case classify.R0:
		return ctx.localRead(a, verdict)
	}
	return verdict(Deny, "13", aerr.PolicyDefault, "no rule allows this action") // rule 13
}

// ownBranches is rule 9: W1 only on branches Athel created for this change.
func (ctx Context) ownBranches(a classify.Action, verdict verdictFn) ActionDecision {
	for _, b := range a.Branches {
		if !slices.Contains(ctx.OwnBranches, b) {
			return verdict(Deny, "9", aerr.PolicyForeignBranch, "pushes to "+b+", which is not a branch Athel created for "+ctx.ChangeID)
		}
	}
	return verdict(Allow, "9", "", "pushes Athel's own branch")
}

// worktreeWrite is rule 10: W0 inside the change's worktree only.
func (ctx Context) worktreeWrite(a classify.Action, verdict verdictFn) ActionDecision {
	for _, p := range a.Paths {
		if !ctx.inWorktree(p) {
			return verdict(Deny, "10", aerr.PolicyOutsideWorktree, "writes "+p+" outside the worktrees of "+ctx.ChangeID)
		}
	}
	return verdict(Allow, "10", "", "writes inside the change's worktree")
}

// localRead is rule 12: R0 inside the workspace is allowed, anywhere else needs a human.
func (ctx Context) localRead(a classify.Action, verdict verdictFn) ActionDecision {
	for _, p := range a.Paths {
		if !ctx.inWorkspace(p) && !ctx.devClone(p) {
			return verdict(Approve, "12", "", "reads "+p+" outside the workspace and the developer's confirmed clones")
		}
	}
	return verdict(Allow, "12", "", "local read inside the workspace or a confirmed developer clone")
}

// toolPolicy applies tools.yaml policy.<env>.<read|write> (allow | approve | deny).
func (ctx Context) toolPolicy(d *ActionDecision, a classify.Action, kind string, verdict verdictFn, rule string) ActionDecision {
	setting := "approve"
	if ctx.Policy != nil {
		if t := ctx.Policy.Tools[a.ToolID]; t != nil {
			if p, ok := t.Policy[d.Env]; ok {
				if kind == "read" && p.Read != "" {
					setting = p.Read
				}
				if kind == "write" && p.Write != "" {
					setting = p.Write
				}
			} else if kind == "read" {
				setting = "allow" // a registered read verb with no per-environment rule
			}
		}
	}
	switch setting {
	case "allow":
		return verdict(Allow, rule, "", fmt.Sprintf("tools.yaml allows %s %s in %s", a.Tool, kind, d.Env))
	case "deny":
		return verdict(Deny, rule, aerr.PolicyToolDenied, fmt.Sprintf("tools.yaml denies %s %s in %s", a.Tool, kind, d.Env))
	}
	return verdict(Approve, rule, "", fmt.Sprintf("%s %s in %s needs a human", a.Tool, kind, d.Env))
}

// planTask finds the approved-plan task that allows a write (doc 04 rule 7).
// Repo-level actions without a path (git push) are matched against the worktree they run in.
func (ctx Context) planTask(a classify.Action, cwd string) (*plan.Task, string) {
	if ctx.Plan == nil {
		return nil, "no approved plan for " + ctx.ChangeID + "; writes need an approved plan task"
	}
	if a.Tier == classify.W2 {
		return ctx.resourceTask(a)
	}
	verb := verbOf(a)
	paths := a.Paths
	if len(paths) == 0 && cwd != "" {
		paths = []string{cwd}
	}
	if len(paths) == 0 {
		return nil, a.Text + " has no target inside a planned repo"
	}
	var task *plan.Task
	for _, p := range paths {
		repo, rel, ok := ctx.worktreeOf(path.Clean(p))
		if !ok {
			return nil, p + " is not inside a worktree of " + ctx.ChangeID
		}
		if repoLevel[verb] {
			rel = ""
		}
		task = ctx.Plan.Match(plan.Write{Repo: repo, Path: rel, Verb: verb})
		if task == nil {
			return nil, fmt.Sprintf("no task of the approved plan allows %s on %s/%s", verb, repo, rel)
		}
	}
	return task, ""
}

// repoLevel verbs act on a whole repository, so only the task's repo and verb must match.
var repoLevel = map[string]bool{"push": true, "commit": true, "test": true, "pr": true, "comment": true}

// resourceTask finds the task that lists a shared-write tool in env_actions or resources.
func (ctx Context) resourceTask(a classify.Action) (*plan.Task, string) {
	for i := range ctx.Plan.Tasks {
		t := &ctx.Plan.Tasks[i]
		for _, name := range []string{a.Tool, a.ToolID} {
			if name != "" && (slices.Contains(t.Resources, name) || slices.Contains(t.EnvActions, name)) {
				return t, ""
			}
		}
	}
	return nil, a.Tool + " is not listed in any task's env_actions or resources"
}

// verbOf maps an action to the plan verbs of doc 05 (edit, create, delete, test, commit, push,
// pr, comment).
func verbOf(a classify.Action) string {
	switch {
	case a.Tool == "gh" && strings.HasSuffix(a.Verb, " comment"):
		return "comment"
	case a.Tool == "gh" && strings.HasPrefix(a.Verb, "pr "):
		return "pr"
	case a.Tier == classify.W1:
		return "push"
	case a.Tool == "Write":
		return "create"
	case a.Tool == "rm" || a.Tool == "rmdir" || a.Tool == "unlink" || a.Tool == "shred":
		return "delete"
	case a.Tool == "git":
		return "commit"
	case slices.Contains([]string{"go", "npm", "pnpm", "yarn", "bun", "cargo", "pip", "pip3", "make", "gradle", "mvn", "pytest", "jest", "uv", "poetry"}, a.Tool):
		return "test"
	}
	return "edit"
}

// worktreeOf maps an absolute path to (repo, path inside the repo) when it is inside one of the
// change's worktrees: <workspace>/repos/<repo>@<change-id>/… (doc 02 §State model).
func (ctx Context) worktreeOf(p string) (string, string, bool) {
	base := path.Join(ctx.WorkspaceRoot, "repos") + "/"
	if ctx.WorkspaceRoot == "" || !strings.HasPrefix(p, base) {
		return "", "", false
	}
	dir, rel, _ := strings.Cut(strings.TrimPrefix(p, base), "/")
	repo, change, ok := strings.Cut(dir, "@")
	if !ok || change != ctx.ChangeID || strings.Contains(rel, "..") {
		return "", "", false
	}
	return repo, rel, true
}

func (ctx Context) inWorktree(p string) bool {
	_, _, ok := ctx.worktreeOf(path.Clean(p))
	return ok
}

func (ctx Context) inWorkspace(p string) bool {
	if ctx.WorkspaceRoot == "" || p == "?" || strings.HasPrefix(p, "$HOME") {
		return false
	}
	clean := path.Clean(p)
	return clean == ctx.WorkspaceRoot || strings.HasPrefix(clean, ctx.WorkspaceRoot+"/")
}

func denyText(why string, code aerr.Code, rule string) string {
	return fmt.Sprintf("Athel denied this: %s [%s, rule %s]", why, code, rule)
}

// reason is shown to the model (deny) or the human (approve, FR-SAFE-005: command, target
// environment, plan task, reason and source).
func (ctx Context) reason(res Result) string {
	var deciding *ActionDecision
	for i := range res.Actions {
		if res.Actions[i].Decision == res.Decision && res.Actions[i].Rule == res.Rule {
			deciding = &res.Actions[i]
			break
		}
	}
	if deciding == nil {
		return "Athel: no decision"
	}
	switch res.Decision {
	case Deny:
		return denyText(deciding.Why, res.Code, res.Rule)
	case Approve:
		task := deciding.Task
		if task == "" {
			task = "none"
		}
		source := "main agent"
		if ctx.Agent != "" {
			source = "subagent " + ctx.Agent
		}
		return fmt.Sprintf("Athel asks for your approval.\ncommand: %s\nenvironment: %s\nplan task: %s\nreason: %s\nsource: proposed by the %s (rule %s)",
			deciding.Action.Text, deciding.Env, task, deciding.Why, source, res.Rule)
	case Allow:
		return "Athel allowed: " + deciding.Why + " (rule " + res.Rule + ")"
	}
	return "Athel: " + deciding.Why
}
