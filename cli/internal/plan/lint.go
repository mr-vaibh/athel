package plan

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Task kinds (plan@2; doc 04 ordering rules, ADR 020).
const (
	KindExpand   = "expand"
	KindMigrate  = "migrate"
	KindConsume  = "consume"
	KindContract = "contract"
	KindInternal = "internal"
)

// LintContext is what the plan is checked against: the acceptance criteria of the change's specs
// and the blast radius risk level ("" before a report exists).
type LintContext struct {
	Criteria []string
	Risk     string
}

var deployRe = regexp.MustCompile(`(?i)\b(deploy|release|roll ?out|kubectl\s+(apply|rollout|set)|helm\s+(install|upgrade)|terraform\s+apply|promote)\b`)

// Lint checks a plan@2 against doc 05's planning rules and FR-PLAN-002/003. Every problem names
// the task and how to fix it.
func Lint(p *Plan, ctx LintContext) []string {
	var problems []string
	problems = append(problems, coverage(p, ctx.Criteria)...)
	problems = append(problems, ordering(p)...)
	for _, t := range p.Tasks {
		problems = append(problems, taskRules(t, ctx.Risk)...)
	}
	return problems
}

// coverage is FR-PLAN-002: every criterion is covered by a task, every task covers a criterion,
// and no task covers a criterion the specs do not have.
func coverage(p *Plan, criteria []string) []string {
	var problems []string
	covered := map[string]bool{}
	for _, t := range p.Tasks {
		if len(t.Covers) == 0 {
			problems = append(problems, t.ID+" covers no acceptance criterion; every task serves at least one (FR-PLAN-002)")
		}
		for _, ac := range t.Covers {
			if !slices.Contains(criteria, ac) {
				problems = append(problems, t.ID+" covers "+ac+", which no spec of the change defines")
			}
			covered[ac] = true
		}
	}
	for _, ac := range criteria {
		if !covered[ac] {
			problems = append(problems, ac+" is covered by no task (FR-PLAN-002)")
		}
	}
	return problems
}

// ordering is FR-PLAN-003, doc 04's expand-and-contract rules: a consumer moves only after a
// provider expanded, and a removal comes last, deferred until every consumer and migration.
func ordering(p *Plan) []string {
	var problems []string
	byKind := func(kind string) []string {
		var out []string
		for _, t := range p.Tasks {
			if t.Kind == kind {
				out = append(out, t.ID)
			}
		}
		return out
	}
	expands := byKind(KindExpand)
	for _, t := range p.Tasks {
		before := p.ancestors(t.ID)
		switch t.Kind {
		case KindConsume:
			if !slices.ContainsFunc(expands, func(e string) bool { return before[e] }) {
				problems = append(problems, t.ID+" moves a consumer but depends on no expand task; the provider adds the new contract first (doc 04 ordering rule 1)")
			}
		case KindContract:
			if !t.Deferred {
				problems = append(problems, t.ID+" removes an old contract; mark it deferred, it runs after consumers are deployed everywhere (doc 04 rule 3)")
			}
			for _, need := range append(byKind(KindConsume), byKind(KindMigrate)...) {
				if !before[need] {
					problems = append(problems, t.ID+" must come after "+need+" (doc 04 rules 3 and 4)")
				}
			}
		}
	}
	return problems
}

// ancestors are every task id depends on, directly or not (the graph is acyclic by Decode).
func (p *Plan) ancestors(id string) map[string]bool {
	deps := map[string][]string{}
	for _, t := range p.Tasks {
		deps[t.ID] = t.DependsOn
	}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(n string) {
		for _, d := range deps[n] {
			if !seen[d] {
				seen[d] = true
				walk(d)
			}
		}
	}
	walk(id)
	return seen
}

// taskRules are doc 05's planning rules 1, 2, 4 and 5 for one task.
func taskRules(t Task, risk string) []string {
	var problems []string
	if t.Size == "L" && t.SizeReason == "" {
		problems = append(problems, t.ID+" is size L without a size_reason; split it or say why it cannot be (planning rule 1)")
	}
	changes := slices.ContainsFunc(t.Verbs, func(v string) bool { return v == "edit" || v == "create" })
	if changes && t.Kind != KindContract && !slices.Contains(t.Verbs, "test") {
		problems = append(problems, t.ID+" changes behaviour without the test verb; tests belong to the task that changes behaviour (planning rule 2)")
	}
	for _, a := range t.EnvActions {
		if deployRe.MatchString(a) {
			problems = append(problems, fmt.Sprintf("%s env action %q deploys; Athel never deploys, the developer does (planning rule 4)", t.ID, a))
		}
	}
	risky := t.Kind == KindMigrate || t.Kind == KindExpand || t.Kind == KindContract
	if (risk == "HIGH" || risk == "CRITICAL") && risky && strings.TrimSpace(t.Rollback) == "" {
		problems = append(problems, t.ID+" ("+t.Kind+") needs a rollback at "+risk+" risk (planning rule 5; doc 05 gates)")
	}
	return problems
}
