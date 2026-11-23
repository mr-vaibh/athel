package plan

import "slices"

// Contract is a changed contract from the blast radius (doc 05 step 2), with the consumers the
// report places inside the change set. The CLI builds it; the plan package does not read reports.
type Contract struct {
	Item      string
	Provider  string // the service that owns the contract
	Change    string // additive, behavioural or breaking
	Consumers []string
}

// setRules are M5's checks on a multi-repo plan (M5-plan §5.2, FR-PLAN-003): every consumer in
// the change set has a task, each consumer moves after its own provider expands, and a breaking
// change has an expand path or a coordinated release (doc 04 rule 5).
func setRules(p *Plan, ctx LintContext) []string {
	same := ctx.SameRepo
	if same == nil {
		same = func(service, repo string) bool { return service == repo }
	}
	var problems []string
	for _, c := range ctx.Contracts {
		if c.Change == "internal" {
			continue
		}
		for _, consumer := range c.Consumers {
			problems = append(problems, consumerRules(p, c, consumer, same, ctx.Coordinated)...)
		}
		if c.Change == "breaking" && len(c.Consumers) > 0 && !ctx.Coordinated && !expandPath(p, c.Provider, same) {
			problems = append(problems, c.Item+" is a breaking change for "+joinAnd(c.Consumers)+
				"; plan it as expand in "+c.Provider+", consume in each consumer, then a deferred contract task, or raise a coordinated-release card (doc 04 rules 1–3 and 5)")
		}
	}
	return problems
}

func consumerRules(p *Plan, c Contract, consumer string, same func(string, string) bool, coordinated bool) []string {
	tasks := tasksIn(p, consumer, same)
	if len(tasks) == 0 {
		return []string{consumer + " consumes " + c.Item + " from " + c.Provider + " and is in the change set, but no task changes it; add its consume task, or take it out of the set in the blast radius"}
	}
	if coordinated || same(consumer, c.Provider) {
		return nil
	}
	var problems []string
	for _, t := range tasks {
		if t.Kind != KindConsume {
			continue
		}
		before := p.ancestors(t.ID)
		if !slices.ContainsFunc(tasksIn(p, c.Provider, same), func(e Task) bool { return e.Kind == KindExpand && before[e.ID] }) {
			problems = append(problems, t.ID+" moves "+consumer+" to "+c.Item+" but depends on no expand task in "+c.Provider+"; the provider adds it first (doc 04 rule 2)")
		}
	}
	return problems
}

// expandPath: the provider expands now and removes the old contract in a deferred task.
func expandPath(p *Plan, provider string, same func(string, string) bool) bool {
	tasks := tasksIn(p, provider, same)
	return slices.ContainsFunc(tasks, func(t Task) bool { return t.Kind == KindExpand }) &&
		slices.ContainsFunc(p.Tasks, func(t Task) bool { return t.Deferred && t.Kind == KindContract && same(provider, t.Repo) })
}

// tasksIn are the built tasks in the repo of service.
func tasksIn(p *Plan, service string, same func(string, string) bool) []Task {
	var out []Task
	for _, t := range p.Built() {
		if same(service, t.Repo) {
			out = append(out, t)
		}
	}
	return out
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	out := items[0]
	for _, s := range items[1 : len(items)-1] {
		out += ", " + s
	}
	return out + " and " + items[len(items)-1]
}
