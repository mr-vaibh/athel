package cli

import (
	"encoding/json"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/engine"
	"github.com/mr-vaibh/athel/cli/internal/envdetect"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

// cmdPolicyDecide prints the decision the PreToolUse hook would make for one tool call
// (debugging and red-team runs), without recording it.
func cmdPolicyDecide(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("policy decide")
	tool := fs.String("tool", "", "Claude Code tool name, e.g. Bash or Edit (required)")
	input := fs.String("input", "", "tool_input as a JSON object (required)")
	agent := fs.String("agent", "", "subagent type proposing the call; empty for the main agent")
	cwd := fs.String("cwd", "", "working directory of the call (default: the current directory)")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	if *tool == "" || *input == "" {
		return nil, usagef("policy decide: --tool and --input are required")
	}
	if !json.Valid([]byte(*input)) {
		return nil, aerr.New(aerr.StateInvalid, "--input is not valid JSON")
	}
	if *cwd == "" {
		*cwd = r.env.Cwd
	}
	if r.workspace == "" {
		r.workspace = *cwd // find the workspace from the call's directory, as the hook does
	}
	return r.decide(classify.Call{Tool: *tool, Input: json.RawMessage(*input), Cwd: *cwd}, r.session, *agent, *agent != ""), nil
}

// decide classifies a tool call and applies the policy rules (doc 04 §Decision order). The
// mode follows the session's change lock (ADR 015). It never fails: when the config, the
// policy files or the state cannot be read, that error becomes rule 0 and every write is
// denied (FR-SAFE-012), because "no decision" would let the call through.
func (r *runCtx) decide(call classify.Call, session, agent string, subagent bool) engine.Result {
	ctx, err := r.engineContext(session, agent)
	ctx.Subagent = subagent
	if err != nil && ctx.PolicyErr == nil {
		ctx.PolicyErr = err
	}
	cr := classify.Classify(call, ctx.Policy, classify.Options{ReadFile: readFile})
	return engine.Decide(call, cr, ctx)
}

// hookDecide adapts decide to the PreToolUse request.
func (r *runCtx) hookDecide(req *hookio.Request) engine.Result {
	call := classify.Call{Tool: req.ToolName, Input: json.RawMessage(req.ToolInput), Cwd: req.Cwd}
	if call.Cwd == "" {
		call.Cwd = r.env.Cwd
	}
	return r.decide(call, req.SessionID, req.AgentType, req.AgentID != "")
}

func (r *runCtx) engineContext(session, agent string) (engine.Context, error) {
	ctx := engine.Context{Mode: engine.Guard, Agent: agent, Headless: r.env.Getenv("ATHEL_HEADLESS") == "1"}
	ctx.Policy, ctx.PolicyErr = r.policySet()
	if ctx.PolicyErr == nil {
		home, err := r.env.UserHome()
		if err != nil {
			return ctx, aerr.Wrap(aerr.StateIO, "find the home folder for environment detection", err)
		}
		ctx.Resolver = &envdetect.Resolver{Policy: ctx.Policy, Home: home, Getenv: r.env.Getenv, ReadFile: readFile}
	}
	ws, err := r.workspaceDir()
	if err != nil {
		return ctx, nil // outside a workspace there is no change, so only the guard rules apply
	}
	ctx.WorkspaceRoot = ws.Root
	s, err := r.storeFor()
	if err != nil {
		return ctx, err
	}
	ctx.HumanGrant = func(change, target string) bool {
		g, err := s.Grants().Valid(change, target, session)
		return err == nil && g != nil
	}
	c, err := s.HeldBy(session)
	if err != nil || c == nil {
		return ctx, err
	}
	ctx.Mode, ctx.ChangeID = engine.Change, c.ID
	if p, ok := s.ApprovedPlan(c); ok {
		ctx.Plan = p
	}
	return ctx, nil
}
