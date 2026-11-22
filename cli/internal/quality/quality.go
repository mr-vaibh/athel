// Package quality is doc 06's code quality standard as rule IDs: the reviewer's checklist and
// the ten standards, each with an ID findings cite (`errors.no-silent-catch`), so patterns can be
// counted and turned into lessons (doc 07). Doc 06 names one ID; the rest follow its form.
package quality

import "slices"

// Rule is one rule of the standard.
type Rule struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Rules are every rule ID a review, QA or security finding may cite, grouped by checklist area.
var Rules = []Rule{
	{"spec.fit", "Each changed behaviour maps to a requirement; nothing extra"},
	{"correctness.requirement-tested", "Every requirement has a test"},
	{"correctness.edge-case", "Edge cases: null and empty, boundaries, concurrency, idempotency, time zones, units"},
	{"errors.no-silent-catch", "No error is swallowed or caught without logging and a typed error"},
	{"errors.typed", "Failures are typed errors with meaningful messages and correct status codes"},
	{"errors.no-magic-return", "No magic return value means failure"},
	{"errors.path-tested", "Every error path is handled and tested"},
	{"boundary.validate-input", "External input is validated at the edge; validated data is trusted inside"},
	{"reliability.idempotent", "Handlers called over the network or by a queue are idempotent and retry-safe"},
	{"contracts.backward-compatible", "Contracts and schemas stay backward compatible unless the plan says otherwise; versioning is respected"},
	{"data.reversible-migration", "Migrations are reversible or flagged"},
	{"data.no-n-plus-one", "No N+1 queries"},
	{"data.index-for-query", "New queries have their indexes"},
	{"observability.structured-logs", "Structured logs at the right level"},
	{"observability.no-pii", "No personal data in logs"},
	{"observability.metrics", "Metrics for new endpoints and queues"},
	{"observability.trace-propagation", "Trace context is passed on"},
	{"security.input-validation", "Input is validated before use"},
	{"security.authz", "Authorization is checked on every new path"},
	{"security.no-secrets", "No secrets in code, config or logs"},
	{"security.least-privilege", "Least privilege and safe defaults"},
	{"security.parameterised-queries", "Queries are parameterised"},
	{"security.output-encoding", "Output is encoded for where it goes"},
	{"security.safe-deserialisation", "Deserialisation is safe"},
	{"security.dependency", "No new vulnerable dependency"},
	{"tests.ac-covered", "Every acceptance criterion has a test that checks it"},
	{"tests.failure-paths", "Failure paths are tested"},
	{"tests.deterministic", "Tests are deterministic: no sleeps, no real network, fixed seeds and clocks"},
	{"tests.isolated", "No shared state or reliance on order between tests"},
	{"tests.repo-style", "Same test framework and style as the repo's existing tests"},
	{"deadweight.no-todo", "No TODOs"},
	{"deadweight.no-commented-code", "No commented-out code"},
	{"deadweight.no-unused-code", "No unused code"},
	{"deadweight.no-debug-output", "No debug output"},
	{"deadweight.unjustified-dependency", "No new dependency without a reason"},
	{"readability.small-functions", "Small functions"},
	{"readability.precise-names", "Precise names"},
	{"readability.why-comments", "Comments explain why, not what"},
	{"readability.conventions", "The repo's conventions win over personal style"},
}

// Known reports whether id is a rule of the standard.
func Known(id string) bool {
	return slices.ContainsFunc(Rules, func(r Rule) bool { return r.ID == id })
}
