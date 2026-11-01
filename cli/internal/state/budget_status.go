package state

// Budget statuses (doc 02 §Runtime model: warn at 80%, block at 100%).
const (
	BudgetOK       = "ok"
	BudgetWarning  = "warning"
	BudgetExceeded = "exceeded"
)
