package cli

import (
	"fmt"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/decisions"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
)

// coordinatedQuestion starts the coordinated-release card's question; coordinated finds the
// card by it, as sizeAllowed does for the size card.
const coordinatedQuestion = "Backward compatibility is not possible for "

// cmdSetCoordinate raises the coordinated-release card (doc 04 ordering rule 5; ADR 037): the
// architect found that a breaking change cannot get an expand path, so the plan says so in a
// card with a coordinated-release proposal instead of shipping it silently. The CLI names the
// breaking contracts from the blast radius; the architect gives the reason.
func cmdSetCoordinate(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("set coordinate")
	reason := fs.String("reason", "", "why no expand path is possible (required)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(*reason) == "" {
		return nil, usagef("set coordinate: --reason is required")
	}
	s, _, err := r.mutating()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	rep, err := r.blastReport(s, c.ID)
	if err != nil {
		return nil, err
	}
	var breaking []plan.Contract
	for _, k := range contracts(rep) {
		if k.Change == "breaking" && len(k.Consumers) > 0 {
			breaking = append(breaking, k)
		}
	}
	if len(breaking) == 0 {
		return nil, aerr.New(aerr.StateInvalid, c.ID+"'s blast radius has no breaking change with consumers in the change set; nothing to coordinate").
			WithNextAction("plan additive changes with expand, consume and a deferred contract task (doc 04 rules 1–3)")
	}
	id, err := r.raiseOnce(c.ID, coordinatedCard(breaking, *reason))
	if err != nil {
		return nil, err
	}
	return map[string]string{"change": c.ID, "card": id}, nil
}

func coordinatedCard(breaking []plan.Contract, reason string) decisions.Draft {
	var items, lines []string
	for _, k := range breaking {
		items = append(items, k.Item)
		lines = append(lines, fmt.Sprintf("%s (%s) is used by %s", k.Item, k.Provider, strings.Join(k.Consumers, ", ")))
	}
	question := clip(coordinatedQuestion+strings.Join(items, ", "), 250) + ". How should Athel ship it?"
	return decisions.Draft{
		Rule: "A2", Step: "plan", Question: question,
		Context: clip("Doc 04 rule 5: when backward compatibility is impossible, Athel proposes a coordinated release and never ships a breaking change silently. "+
			strings.Join(lines, "; ")+". Why no expand path: "+reason, maxCardContext),
		Options: []decisions.Option{
			{Key: "a", Label: "Coordinated release: merge every PR together, in the stated order, in one deploy window",
				Consequence: "the states between merges are not supported: the PRs say so, and each carries its rollback; Athel rehearses the final state"},
			{Key: "b", Label: "Re-plan with an expand path (add the new contract, move consumers, remove the old one later)",
				Consequence: "every merge stays safe on its own; the change takes longer and the old contract lives until a follow-up change"},
		},
		Recommendation: decisions.Recommendation{Option: "a", Reason: clip("the architect found no expand path: "+reason, maxCardContext)},
		Default:        "b", DefaultSafe: true, Blocks: []string{"plan"},
	}
}

// coordinated reports whether the developer chose the coordinated release for change id.
func (r *runCtx) coordinated(id string) (bool, error) {
	f, err := r.readDecisions(id)
	if err != nil {
		return false, err
	}
	for _, e := range f.Decisions {
		if c := e.Card; c != nil && c.Answer != nil && c.Answer.Option == "a" && strings.HasPrefix(c.Question, coordinatedQuestion) {
			return true, nil
		}
	}
	return false, nil
}
