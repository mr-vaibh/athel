package state

import (
	"fmt"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// DefaultWarnRatio is the budget warning threshold (doc 02: "At 80% the conductor writes a
// warning").
const DefaultWarnRatio = 0.8

// Usage is an increment of budget usage.
type Usage struct {
	CostUSD   float64
	Minutes   float64
	ToolCalls int
}

var budgetRank = map[string]int{BudgetOK: 0, BudgetWarning: 1, BudgetExceeded: 2}

// dimensionStatus grades used against limit. A zero limit means "no limit" so a change
// created without a budget never reports a false warning.
func dimensionStatus(used, limit, warn float64) string {
	switch {
	case limit <= 0:
		return BudgetOK
	case used >= limit:
		return BudgetExceeded
	case used >= warn*limit:
		return BudgetWarning
	}
	return BudgetOK
}

type dimension struct {
	name        string
	used, limit float64
}

func dimensions(b Budget) []dimension {
	return []dimension{
		{"cost", b.UsedCostUSD, b.MaxCostUSD},
		{"time", b.UsedMinutes, b.MaxHours * 60},
		{"tool_calls", float64(b.UsedToolCalls), float64(b.MaxToolCalls)},
	}
}

// RecordUsage adds u to change id's budget, recomputes the status and appends an event for
// every dimension that crossed into a worse status. M0 records only; nothing blocks on it.
func (s *Store) RecordUsage(id, session string, u Usage) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	if u.CostUSD < 0 || u.Minutes < 0 || u.ToolCalls < 0 {
		return nil, aerr.New(aerr.StateInvalid, fmt.Sprintf("budget usage cannot be negative: %+v", u))
	}
	warn := s.warnRatio()
	return s.Update(id, func(c *Change) error {
		now := s.Clock.Now()
		if err := s.claim(c, session, now); err != nil {
			return err
		}
		before := dimensions(c.Budget)
		c.Budget.UsedCostUSD += u.CostUSD
		c.Budget.UsedMinutes += u.Minutes
		c.Budget.UsedToolCalls += u.ToolCalls
		worst := BudgetOK
		for i, d := range dimensions(c.Budget) {
			old, cur := dimensionStatus(before[i].used, before[i].limit, warn), dimensionStatus(d.used, d.limit, warn)
			if budgetRank[cur] > budgetRank[old] {
				c.Budget.Events = append(c.Budget.Events, BudgetEvent{At: clock.Format(now), Status: cur, Dimension: d.name})
			}
			if budgetRank[cur] > budgetRank[worst] {
				worst = cur
			}
		}
		c.Budget.Status = worst
		return nil
	})
}

func (s *Store) warnRatio() float64 {
	if s.WarnRatio > 0 {
		return s.WarnRatio
	}
	return DefaultWarnRatio
}
