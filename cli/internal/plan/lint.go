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
	// Exists reports whether repo has path on its base branch, and whether that is known (Athel
	// has the repo's cache clone). Nil checks nothing.
	Exists func(repo, path string) (exists, known bool)
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
		problems = append(problems, newFiles(t, ctx.Exists)...)
	}
	return problems
}

// newFiles: a task naming a file its repo does not have yet needs the create verb, or the build
// stops at the first write and the change goes back to planning. Globs are not checked.
func newFiles(t Task, exists func(repo, path string) (bool, bool)) []string {
	if exists == nil || slices.Contains(t.Verbs, "create") {
		return nil
	}
	var problems []string
	for _, p := range t.Paths {
		if strings.ContainsAny(p, "*?[") {
			continue
		}
		if ok, known := exists(t.Repo, p); known && !ok {
			problems = append(problems, t.ID+" names "+p+", which "+t.Repo+" does not have yet, without the create verb; add create and edit")
		}
	}
	return problems
}

// coverage is FR-PLAN-002: every criterion is covered by a task, every task covers a criterion,
// no task covers a criterion the specs do not have, and every criterion is covered by a task that
// changes code (ADR 027).
func coverage(p *Plan, criteria []string) []string {
	var problems []string
	covered, tested := map[string]bool{}, map[string]bool{}
	for _, t := range p.Tasks {
		for _, ac := range t.Covers {
			tested[ac] = tested[ac] || len(t.Paths) > 0
		}
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
		switch {
		case !covered[ac]:
			problems = append(problems, ac+" is covered by no task (FR-PLAN-002)")
		case !tested[ac]:
			// A task with no paths changes no code, so it cannot hold the test that proves the
			// criterion (doc 06: every requirement has a test; ADR 027).
			problems = append(problems, ac+" is covered only by tasks that change no code; a task with paths must cover it, with its test")
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

// verbRules: a task that changes code carries test (planning rule 2), and one that creates files
// carries edit, because the review loop fixes its findings inside the task by editing what it
// created (ADR 030).
func verbRules(t Task) []string {
	var problems []string
	changes := slices.ContainsFunc(t.Verbs, func(v string) bool { return v == "edit" || v == "create" })
	if changes && t.Kind != KindContract && !slices.Contains(t.Verbs, "test") {
		problems = append(problems, t.ID+" changes behaviour without the test verb; tests belong to the task that changes behaviour (planning rule 2)")
	}
	if slices.Contains(t.Verbs, "create") && !slices.Contains(t.Verbs, "edit") {
		problems = append(problems, t.ID+" creates files without the edit verb; its review fixes and QA tests edit them, so add edit (ADR 030)")
	}
	return problems
}

// taskRules are doc 05's planning rules 1, 2, 4 and 5 for one task.
func taskRules(t Task, risk string) []string {
	var problems []string
	if t.Size == "L" && t.SizeReason == "" {
		problems = append(problems, t.ID+" is size L without a size_reason; split it or say why it cannot be (planning rule 1)")
	}
	problems = append(problems, verbRules(t)...)
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
