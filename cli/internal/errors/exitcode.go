package errors

// Process exit codes. They are part of the CLI contract (docs/plans/M0-plan.md §4.2):
// skills and scripts branch on them, so they never change.
const (
	ExitOK        = 0
	ExitInternal  = 1
	ExitUsage     = 2
	ExitPolicy    = 10
	ExitAuth      = 11
	ExitTool      = 12
	ExitState     = 13
	ExitGate      = 14
	ExitBudget    = 15
	ExitConflict  = 16
	ExitIntegrity = 17
)

var exitByFamily = map[Family]int{
	FamilyPolicy:    ExitPolicy,
	FamilyAuth:      ExitAuth,
	FamilyTool:      ExitTool,
	FamilyState:     ExitState,
	FamilyGate:      ExitGate,
	FamilyBudget:    ExitBudget,
	FamilyConflict:  ExitConflict,
	FamilyIntegrity: ExitIntegrity,
}

// ExitCode maps an error to the process exit code. A nil error is success; an error that
// carries no Athel code is an internal error, because every expected failure must be typed.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	e, ok := As(err)
	if !ok {
		return ExitInternal
	}
	if code, known := exitByFamily[e.Code.Family()]; known {
		return code
	}
	return ExitInternal
}
