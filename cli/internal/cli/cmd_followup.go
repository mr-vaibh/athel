package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	"github.com/mr-vaibh/athel/cli/internal/changeset"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/grants"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// The deferred cleanup (doc 04 rule 3; M5-plan G3, A6): removing the old contract is a separate,
// later change, raised only after consumers are deployed everywhere. Every PR of the set must be
// merged (Athel checks the host), and the developer's own /athel:followup says the consumers are
// deployed; Athel then drafts the follow-up's PRD for the developer to start. M6 adds the traffic
// check (doc 07 contract cleanup check).

type followupDraft struct {
	Change string `json:"change"`
	Path   string `json:"path"`
	Next   string `json:"next"`
}

func cmdFollowupDraft(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("followup draft"), args, 1)
	if err != nil {
		return nil, err
	}
	s, session, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	p, err := r.cleanupReady(s, c)
	if err != nil {
		return nil, err
	}
	g, err := s.Grants().Consume(c.ID, grants.TargetFollowup, session)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, aerr.New(aerr.PolicyHumanOrigin, "drafting "+c.ID+"'s deferred cleanup needs the developer's own prompt /athel:followup "+c.ID+" in this session").
			WithNextAction("ask the developer to confirm every consumer is deployed and type /athel:followup " + c.ID)
	}
	if _, err := s.SaveChangeFile(c.ID, session, "followup.md", []byte(followupPRD(c, p))); err != nil {
		return nil, err
	}
	if _, err := s.AuditLog().Append(audit.Record{Kind: audit.KindGrantUsed, Session: audit.Str(session), Change: audit.Str(c.ID),
		To: audit.Str(grants.TargetFollowup), By: audit.Str(state.ByHuman), Reason: audit.Str("grant " + g.ID + " for followup.md"), Result: audit.ResultOK}); err != nil {
		return nil, err
	}
	path := filepath.Join(s.WS.ChangeDir(c.ID), "followup.md")
	return followupDraft{Change: c.ID, Path: path, Next: "/athel:start " + path}, nil
}

// cleanupReady: the set has a deferred cleanup and the host shows every PR of the set merged.
func (r *runCtx) cleanupReady(s *state.Store, c *state.Change) (*plan.Plan, error) {
	p, ok := s.ApprovedPlan(c)
	if !ok || c.ChangeSet == nil || !hasCleanup(c.ChangeSet) {
		return nil, aerr.New(aerr.StateInvalid, c.ID+" has no deferred cleanup in its approved change set; nothing to follow up")
	}
	now, source, note := r.hostPRs(c)
	if source != "host" {
		return nil, aerr.New(aerr.ToolUnreachable, "cannot confirm the PRs of "+c.ID+" are merged: "+note).WithNextAction("retry when the host answers")
	}
	var open []string
	for _, e := range c.ChangeSet.Built() {
		if now[e.Repo].State != "merged" {
			open = append(open, e.Repo)
		}
	}
	if len(open) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "the cleanup comes after every PR of "+c.ID+" is merged and deployed; not merged yet: "+strings.Join(open, ", ")).
			WithNextAction("athel set status " + c.ID)
	}
	return p, nil
}

func hasCleanup(set *changeset.Set) bool {
	for _, e := range set.Repos {
		if e.Deferred {
			return true
		}
	}
	return false
}

// followupPRD is the follow-up change's PRD: the deferred tasks, as written in the parent's plan,
// with the checks that make removing the old contract safe.
func followupPRD(c *state.Change, p *plan.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Remove what %s kept for backward compatibility\n\n", c.ID)
	fmt.Fprintf(&b, "Follow-up of %s (%s). Every PR of that change is merged, and the developer confirmed that "+
		"every consumer is deployed (/athel:followup).\n\n", c.ID, c.Title)
	b.WriteString("## Goal\n\nRemove the old contract that the expand-and-contract change left in place, now that no consumer needs it.\n\n")
	b.WriteString("## Requirements\n\n")
	for _, t := range p.Tasks {
		if t.Deferred {
			fmt.Fprintf(&b, "- %s, in %s (paths: %s). Planned in %s as %s.\n", t.Title, t.Repo, strings.Join(t.Paths, ", "), c.ID, t.ID)
		}
	}
	b.WriteString("\n## Acceptance criteria\n\n")
	b.WriteString("- Given every consumer runs on the new contract, when the old one is removed, then every consumer's tests and the " +
		"system check pass without it.\n")
	b.WriteString("- Given the old path, when its traffic is checked over the last 7 days in every environment, then it is zero " +
		"before the removal is merged (doc 07 contract cleanup check; the developer checks it until Athel's verification step does).\n")
	fmt.Fprintf(&b, "\n## Out of scope\n\nAnything else in %s.\n", c.ID)
	return b.String()
}
