// Package classify turns one tool call into the atomic actions it performs, each with a doc 04
// permission tier (R0 … X), the targets it touches and the hints needed to resolve its
// environment. It knows tools; it does not decide. The policy engine applies the rules.
//
// Safety-critical meanings (secret reads, force pushes, merges, deploys, Raptor, database
// writes) are fixed here in code: tools.yaml can refine verbs for rules 8 and 11 but can never
// make a forbidden or irreversible action look harmless.
package classify

import (
	"encoding/json"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/policy"
)

// Tier is a doc 04 permission tier.
type Tier string

// Tiers, from least to most dangerous.
const (
	R0 Tier = "R0" // read, local
	R1 Tier = "R1" // read, remote
	W0 Tier = "W0" // write, local and owned
	W1 Tier = "W1" // write, remote and owned (push own branch, own PR)
	W2 Tier = "W2" // write, shared (tickets, chat)
	W3 Tier = "W3" // write, irreversible or broad (deploys, infra, deletes, DB writes, Raptor)
	X  Tier = "X"  // forbidden (secret reads, force push, protected push, merge, history rewrite)
)

var tierRank = map[Tier]int{R0: 0, R1: 1, W0: 2, W1: 3, W2: 4, W3: 5, X: 6}

// Rank orders tiers by danger.
func Rank(t Tier) int { return tierRank[t] }

// IsWrite reports whether the tier changes anything.
func IsWrite(t Tier) bool { return tierRank[t] >= tierRank[W0] }

// Kind of uncertainty about an action.
const (
	SureKind       = ""
	UnknownCommand = "unknown-command" // parsed fine, but Athel does not know what it does
	Unresolved     = "unresolved"      // a script or make target that could not be read
)

// EnvHints are the environment selectors found in the call itself (doc 04 resolution steps
// 1 and 2). A value of "?" means the selector is present but its value is unknown.
type EnvHints struct {
	Kube    string // kube context
	KubeCfg string // kubeconfig path
	Project string // gcloud project
	GConfig string // gcloud configuration name
	Profile string // AWS profile
	Account string // AWS account
	URLs    []string
}

// Action is one atomic thing a tool call does.
type Action struct {
	Tool      string     `json:"tool"`              // Bash, Edit, kubectl, mcp__grafana__query …
	ToolID    string     `json:"tool_id,omitempty"` // tools.yaml id when registered
	Verb      string     `json:"verb,omitempty"`
	Tier      Tier       `json:"tier"`
	Text      string     `json:"text"`                // human-readable command or call
	Paths     []string   `json:"paths,omitempty"`     // filesystem paths read or written
	Branches  []string   `json:"branches,omitempty"`  // git branches pushed to
	Remote    bool       `json:"remote,omitempty"`    // reaches outside this machine (env applies)
	Forbidden string     `json:"forbidden,omitempty"` // why the action is tier X
	Uncertain string     `json:"uncertain,omitempty"` // UnknownCommand | Unresolved
	Athel     *AthelCall `json:"athel,omitempty"`
	Env       EnvHints   `json:"-"`
	Via       []string   `json:"via,omitempty"`
}

// AthelCall describes an `athel` CLI call (its writes go to .athel/ through the CLI, the
// rule 7 exception), and whether it asks for a human-only transition.
type AthelCall struct {
	Command string `json:"command"`
	Change  string `json:"change,omitempty"`
	To      string `json:"to,omitempty"`
	ByHuman bool   `json:"by_human,omitempty"`
}

// Call is a tool call as the policy engine sees it.
type Call struct {
	Tool  string          // Claude Code tool name
	Input json.RawMessage // tool_input
	Cwd   string
}

// Result is the classification of one call.
type Result struct {
	Actions     []Action `json:"actions"`
	Unparseable bool     `json:"unparseable"`
	Reasons     []string `json:"reasons,omitempty"`
	Malformed   string   `json:"malformed,omitempty"`
}

// Options gives the classifier read access to scripts, Makefiles and git metadata.
type Options struct {
	ReadFile func(path string) ([]byte, error) // relative paths are relative to Call.Cwd
}

// readOnlyTools never touch anything outside Claude Code itself.
var controlTools = map[string]bool{
	"Agent": true, "Task": true, "Workflow": true, "Skill": true, "AskUserQuestion": true,
	"ExitPlanMode": true, "TodoWrite": true, "TaskCreate": true, "TaskUpdate": true, "TaskList": true,
	"TaskGet": true, "TaskOutput": true, "TaskStop": true, "Monitor": true, "BashOutput": true, "KillShell": true,
	"KillBash": true, "ListMcpResourcesTool": true, "ReadMcpResourceTool": true, "ToolSearch": true,
}

// toolHandlers classify Claude Code's built-in tools by name.
var toolHandlers = map[string]func(Call, *policy.Set, Options) Result{
	"Bash": classifyBash,
	"PowerShell": func(c Call, _ *policy.Set, _ Options) Result {
		return Result{Actions: []Action{{Tool: c.Tool, Tier: W0, Text: inputString(c.Input, "command")}}, Unparseable: true,
			Reasons: []string{"PowerShell commands are not parsed yet"}}
	},
	"Read":         fileHandler(R0, "file_path"),
	"NotebookRead": fileHandler(R0, "file_path"),
	"Glob":         fileHandler(R0, "path"),
	"Grep":         fileHandler(R0, "path"),
	"LS":           fileHandler(R0, "path"),
	"Write":        fileHandler(W0, "file_path"),
	"Edit":         fileHandler(W0, "file_path"),
	"MultiEdit":    fileHandler(W0, "file_path"),
	"NotebookEdit": fileHandler(W0, "notebook_path"),
	"WebFetch": func(c Call, _ *policy.Set, _ Options) Result {
		u := inputString(c.Input, "url")
		return Result{Actions: []Action{{Tool: c.Tool, Tier: R1, Text: "fetch " + u, Remote: true, Env: EnvHints{URLs: []string{u}}}}}
	},
	"WebSearch": func(c Call, _ *policy.Set, _ Options) Result {
		return Result{Actions: []Action{{Tool: c.Tool, Tier: R1, Text: "search " + inputString(c.Input, "query"), Remote: true}}}
	},
}

func fileHandler(tier Tier, key string) func(Call, *policy.Set, Options) Result {
	return func(c Call, _ *policy.Set, _ Options) Result { return fileTool(c, tier, key) }
}

// Classify turns a call into actions.
func Classify(c Call, pol *policy.Set, opts Options) Result {
	if h, ok := toolHandlers[c.Tool]; ok {
		return h(c, pol, opts)
	}
	switch {
	case strings.HasPrefix(c.Tool, "mcp__"):
		return Result{Actions: []Action{classifyMCP(c, pol)}}
	case controlTools[c.Tool]:
		return Result{Actions: []Action{{Tool: c.Tool, Tier: R0, Text: c.Tool}}}
	}
	return Result{Actions: []Action{{Tool: c.Tool, Tier: W0, Text: c.Tool, Uncertain: UnknownCommand}}}
}

func inputString(raw json.RawMessage, key string) string {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// fileTool classifies Read/Edit/Write-style tools by the path they touch. Reading (or editing,
// which reads) a secret file is tier X (FR-SAFE-007).
func fileTool(c Call, tier Tier, key string) Result {
	p := inputString(c.Input, key)
	if p == "" && key == "path" {
		p = "."
	}
	if p == "" {
		return Result{Malformed: c.Tool + " tool_input has no " + key}
	}
	a := Action{Tool: c.Tool, Tier: tier, Text: c.Tool + " " + p, Paths: []string{absolute(c.Cwd, p)}}
	if IsWrite(tier) {
		a = stateFileGuard(a)
	}
	if c.Tool != "Write" && c.Tool != "Glob" && IsSecretPath(p) {
		a.Tier, a.Forbidden = X, "reads a secret file ("+p+")"
	}
	return Result{Actions: []Action{a}}
}

func classifyMCP(c Call, pol *policy.Set) Action {
	server, tool := splitMCP(c.Tool)
	a := Action{Tool: c.Tool, Verb: tool, Text: c.Tool, Remote: true}
	t := mcpTool(pol, server, tool)
	if t == nil {
		a.Tier, a.Forbidden = X, "MCP tool "+c.Tool+" is not registered in tools.yaml (FR-DISC-007)"
		return a
	}
	a.ToolID = t.ID
	switch {
	case containsVerb(t.Write, tool):
		a.Tier = W2
	case containsVerb(t.Read, tool):
		a.Tier = R1
	default:
		a.Tier = W2 // a registered server's tool with no declared verb counts as a write
	}
	return a
}

// splitMCP splits mcp__<server>__<tool>; plugin servers look like mcp__plugin_<p>_<s>__<tool>.
func splitMCP(name string) (string, string) {
	rest := strings.TrimPrefix(name, "mcp__")
	i := strings.LastIndex(rest, "__")
	if i < 0 {
		return rest, ""
	}
	return rest[:i], rest[i+2:]
}

func mcpTool(pol *policy.Set, server, tool string) *policy.Tool {
	if pol == nil {
		return nil
	}
	for _, t := range pol.Tools {
		if t.Kind != "mcp" || t.Match.Server != server {
			continue
		}
		if len(t.Match.Tools) == 0 || containsVerb(t.Match.Tools, tool) {
			return t
		}
	}
	return nil
}

func containsVerb(list []string, verb string) bool {
	for _, v := range list {
		if v == "*" || v == verb {
			return true
		}
	}
	return false
}
