package config

import "time"

// Settings is the typed view of the effective athel.yaml that the rest of the CLI uses.
type Settings struct {
	BudgetMaxCostUSD   float64
	BudgetMaxHours     float64
	BudgetMaxToolCalls int
	WarnRatio          float64
	LockTTLMin         int
	HeartbeatEvery     time.Duration
	Identity           string
	Features           map[string]bool
	FactWindows        map[string]time.Duration // facts.freshness, per class
	FactArchiveAfter   time.Duration
}

// Settings extracts typed values. The schema and the built-in layer guarantee every key the
// CLI depends on is present with the right type.
func (r *Result) Settings() Settings {
	v := r.Effective.Values
	budgets, _ := v["budgets"].(map[string]any)
	defaults, _ := budgets["default"].(map[string]any)
	locks, _ := v["locks"].(map[string]any)
	s := Settings{
		BudgetMaxCostUSD:   num(defaults["max_cost_usd"]),
		BudgetMaxHours:     num(defaults["max_hours"]),
		BudgetMaxToolCalls: int(num(defaults["max_tool_calls"])),
		WarnRatio:          num(budgets["warn_ratio"]),
		LockTTLMin:         int(num(locks["ttl_min"])),
		HeartbeatEvery:     time.Duration(num(locks["heartbeat_min"])) * time.Minute,
		Features:           map[string]bool{},
		FactWindows:        map[string]time.Duration{},
	}
	facts, _ := v["facts"].(map[string]any)
	freshness, _ := facts["freshness"].(map[string]any)
	for class, minutes := range freshness {
		s.FactWindows[class] = time.Duration(num(minutes)) * time.Minute
	}
	s.FactArchiveAfter = time.Duration(num(facts["archive_after_days"])) * 24 * time.Hour
	s.Identity, _ = v["identity"].(string)
	features, _ := v["features"].(map[string]any)
	for k, val := range features {
		s.Features[k], _ = val.(bool)
	}
	return s
}

func num(v any) float64 {
	n, _ := number(v)
	return n
}
