package m3

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

// Artifacts are what one run left in its change folder. A result keeps them, so a grader fix can
// regrade the run without paying for it again.
type Artifacts struct {
	State     string `json:"state"`
	Review    string `json:"prd_review"`
	Decisions string `json:"decisions"`
	Blast     string `json:"blast_radius"`
	Plan      string `json:"plan"`
	Trace     string `json:"traceability"`
}

// Grade is one run against its case. EC-1 is Pass; the counts feed EC-2 and the reported
// precision and card figures (M3 plan EC-1, EC-2).
type Grade struct {
	Planned       bool     `json:"planned"`
	TraceComplete bool     `json:"trace_complete"`
	Risk          string   `json:"risk"`
	RiskOK        bool     `json:"risk_ok"`
	KindsOK       bool     `json:"kinds_ok"`
	Expected      int      `json:"expected_items"`
	Found         int      `json:"found_items"`
	ReportItems   int      `json:"report_items"`
	ReportMatched int      `json:"report_items_expected"`
	ExpectedRules int      `json:"expected_card_rules"`
	RulesFound    int      `json:"card_rules_found"`
	Cards         int      `json:"cards"`
	CardsExpected int      `json:"cards_on_expected_rules"`
	ReviewOK      bool     `json:"review_ok"`
	Problems      []string `json:"problems"`
}

// Pass is EC-1: the change reached planned through the step guards (schemas, AC and EARS lints,
// plan lints), its trace is complete, and the risk and task kinds are the expected ones.
func (g Grade) Pass() bool {
	return g.Planned && g.TraceComplete && g.RiskOK && g.KindsOK
}

var levels = []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}

// idFields are the fields that name an entry of each report section. Evidence is left out: a
// reference that merely mentions a service must not count as finding it.
var idFields = map[string][]string{
	"direct":    {"item", "service"},
	"consumers": {"consumer", "of"},
	"data":      {"table", "service"},
	"infra":     {"item"},
	"people":    {"service", "owners", "active_work"},
	"runtime":   {"service", "alerts"},
}

// GradeRun grades one run's artifacts against its case.
func GradeRun(c Case, a Artifacts) Grade {
	g := Grade{Problems: []string{}}
	g.Planned = a.State == "planned" || a.State == "plan_approved"
	if !g.Planned {
		g.Problems = append(g.Problems, "the change is "+orNone(a.State)+", not planned")
	}
	first, _, _ := strings.Cut(a.Trace, "\n")
	g.TraceComplete = strings.HasSuffix(first, "· complete")
	if !g.TraceComplete {
		g.Problems = append(g.Problems, "traceability is not complete")
	}
	g.gradeBlast(c.Expect, a.Blast)
	g.gradePlan(c.Expect, a.Plan)
	g.gradeCards(c.Expect, a.Decisions)
	g.gradeReview(c.Expect, a.Review)
	return g
}

func orNone(s string) string {
	if s == "" {
		return "missing"
	}
	return s
}

func (g *Grade) gradeBlast(e Expect, md string) {
	for _, items := range e.Affected {
		g.Expected += len(items)
	}
	var report map[string]any
	block, ok := evalkit.LastYAMLBlock([]byte(md))
	if !ok || yaml.Unmarshal(block, &report) != nil {
		g.Problems = append(g.Problems, "no planned blast radius report")
		return
	}
	g.Risk, _ = report["risk"].(string)
	g.RiskOK = riskMatches(e, g.Risk)
	if !g.RiskOK {
		g.Problems = append(g.Problems, "risk "+orNone(g.Risk)+", want "+e.Risk+e.RiskAtLeast)
	}
	for section, items := range e.Affected {
		entries := sectionEntries(report, section)
		g.ReportItems += len(entries)
		for _, aliases := range items {
			if slices.ContainsFunc(entries, func(en string) bool { return matches(en, aliases) }) {
				g.Found++
			} else {
				g.Problems = append(g.Problems, section+": "+aliases[0]+" not found")
			}
		}
		for _, en := range entries {
			if slices.ContainsFunc(items, func(aliases []string) bool { return matches(en, aliases) }) {
				g.ReportMatched++
			}
		}
	}
}

// sectionEntries are a report section's entries, each as the lower-case JSON of its naming fields.
func sectionEntries(report map[string]any, section string) []string {
	list, _ := report[section].([]any)
	var out []string
	for _, item := range list {
		entry, _ := item.(map[string]any)
		named := map[string]any{}
		for _, f := range idFields[section] {
			named[f] = entry[f]
		}
		b, _ := json.Marshal(named)
		out = append(out, strings.ToLower(string(b)))
	}
	return out
}

func matches(entry string, aliases []string) bool {
	return slices.ContainsFunc(aliases, func(a string) bool { return strings.Contains(entry, strings.ToLower(a)) })
}

func riskMatches(e Expect, risk string) bool {
	if e.Risk != "" {
		return risk == e.Risk
	}
	return slices.Index(levels, risk) >= slices.Index(levels, e.RiskAtLeast)
}

type planTask struct {
	Kind string `yaml:"kind"`
}

func (g *Grade) gradePlan(e Expect, plan string) {
	var p struct {
		Tasks []planTask `yaml:"tasks"`
	}
	if yaml.Unmarshal([]byte(plan), &p) != nil || len(p.Tasks) == 0 {
		g.Problems = append(g.Problems, "no plan")
		return
	}
	g.KindsOK = true
	for _, k := range e.Ordering {
		if !slices.ContainsFunc(p.Tasks, func(t planTask) bool { return t.Kind == k }) {
			g.KindsOK = false
			g.Problems = append(g.Problems, "the plan has no "+k+" task")
		}
	}
}

func (g *Grade) gradeCards(e Expect, decisions string) {
	var f struct {
		Decisions []struct {
			Kind string `yaml:"kind"`
			Rule string `yaml:"rule"`
		} `yaml:"decisions"`
	}
	_ = yaml.Unmarshal([]byte(decisions), &f)
	g.ExpectedRules = len(e.CardRules)
	var rules []string
	for _, d := range f.Decisions {
		if d.Kind != "card" {
			continue
		}
		g.Cards++
		rules = append(rules, d.Rule)
		if slices.Contains(e.CardRules, d.Rule) {
			g.CardsExpected++
		}
	}
	for _, r := range e.CardRules {
		if slices.Contains(rules, r) {
			g.RulesFound++
		} else {
			g.Problems = append(g.Problems, "no card for ask rule "+r)
		}
	}
}

func (g *Grade) gradeReview(e Expect, md string) {
	var r struct {
		Checklist map[string]struct {
			Status string `yaml:"status"`
		} `yaml:"checklist"`
	}
	block, ok := evalkit.LastYAMLBlock([]byte(md))
	if !ok || yaml.Unmarshal(block, &r) != nil || len(r.Checklist) == 0 {
		g.Problems = append(g.Problems, "no PRD review")
		return
	}
	g.ReviewOK = true
	for _, k := range e.ReviewMissing {
		if st := r.Checklist[k].Status; st != "missing" && st != "partial" {
			g.ReviewOK = false
			g.Problems = append(g.Problems, "the review marks "+k+" "+orNone(st)+", want missing or partial")
		}
	}
}

// Summary is a run's totals against EC-1 (every run valid) and EC-2 (blast recall ≥ 95%).
type Summary struct {
	Runs          int     `json:"runs"`
	Valid         int     `json:"runs_valid"`
	Recall        float64 `json:"blast_recall"`
	Precision     float64 `json:"blast_precision"`
	CardRecall    float64 `json:"card_recall"`
	CardPrecision float64 `json:"card_precision"`
	ReviewOK      int     `json:"runs_review_ok"`
	CostUSD       float64 `json:"cost_usd"`
	DurationS     float64 `json:"duration_s"`
	Met           bool    `json:"exit_criteria_met"`
}

func summarise(cases []Case, results map[string][]Result) Summary {
	var s Summary
	var expected, found, items, matched, rules, rulesFound, cards, cardsExpected int
	for _, c := range cases {
		for _, r := range results[c.ID] {
			g := r.Grade
			s.Runs++
			if r.Pass {
				s.Valid++
			}
			if g.ReviewOK {
				s.ReviewOK++
			}
			expected, found, items, matched = expected+g.Expected, found+g.Found, items+g.ReportItems, matched+g.ReportMatched
			rules, rulesFound, cards, cardsExpected = rules+g.ExpectedRules, rulesFound+g.RulesFound, cards+g.Cards, cardsExpected+g.CardsExpected
			for _, sess := range r.Sessions {
				s.CostUSD += sess.CostUSD
				s.DurationS += sess.Duration
			}
		}
	}
	s.Recall, s.Precision = evalkit.Ratio(found, expected), evalkit.Ratio(matched, items)
	s.CardRecall, s.CardPrecision = evalkit.Ratio(rulesFound, rules), evalkit.Ratio(cardsExpected, cards)
	s.Met = s.Valid == s.Runs && s.Recall >= 0.95
	return s
}

// The grader's own tests use a small report in the CLI's rendered form.

const sampleBlast = "# Blast radius\n\n## Report\n\n```yaml\n" + `schema: athel/blast-radius@1
mode: planned
direct:
  - {item: "PUT /v1/users/{id}/phone", service: users, evidence: [{kind: spec, ref: REQ-1}]}
consumers:
  - {consumer: notifications, of: users.updated.v1, evidence: [{kind: code, ref: orders/main.go:1}]}
  - {consumer: billing, of: users.updated.v1}
data:
  - {table: users, service: users}
people:
  - {service: users, owners: [identity], active_work: ["PR #812 teammate/profile-phone"]}
risk: HIGH
` + "```\n"

const sampleReview = "# PRD review\n\n```yaml\nchecklist:\n  scope: {status: missing}\n  acceptance_criteria: {status: present}\n```\n"

const sampleDecisions = `decisions:
  - {kind: card, rule: A1}
  - {kind: card, rule: A3}
  - {kind: decision, rule: ""}
`

func phoneCase() Case {
	return Case{ID: "phone", Expect: Expect{
		Affected: map[string][][]string{
			"direct":    {{"users", "user-svc"}},
			"consumers": {{"notifications"}, {"orders"}},
			"data":      {{"users.phone", "users"}},
			"people":    {{"812"}},
		},
		Risk: "HIGH", CardRules: []string{"A1", "A2"}, ReviewMissing: []string{"scope"}, Ordering: []string{"migrate"},
	}}
}

func TestGraderBlastRecall(t *testing.T) {
	g := GradeRun(phoneCase(), Artifacts{Blast: sampleBlast})
	// orders appears only in notifications' evidence, which must not count as naming it.
	if g.Expected != 5 || g.Found != 4 || g.ReportItems != 5 || g.ReportMatched != 4 {
		t.Errorf("recall and precision counts: %+v", g)
	}
	if !g.RiskOK || g.Risk != "HIGH" || !slices.Contains(g.Problems, "consumers: orders not found") {
		t.Errorf("risk and problems: %+v", g)
	}
	if g := GradeRun(phoneCase(), Artifacts{Blast: "no block"}); g.Found != 0 || g.Expected != 5 || g.RiskOK || !slices.Contains(g.Problems, "no planned blast radius report") {
		t.Errorf("a missing report finds nothing: %+v", g)
	}
	if g := GradeRun(phoneCase(), Artifacts{Blast: strings.Replace(sampleBlast, "risk: HIGH", "risk: null", 1)}); g.RiskOK {
		t.Error("an uncomputed risk is not the expected one")
	}
}

func TestRiskMatches(t *testing.T) {
	for _, tc := range []struct {
		e    Expect
		risk string
		want bool
	}{
		{Expect{Risk: "HIGH"}, "HIGH", true},
		{Expect{Risk: "HIGH"}, "CRITICAL", false},
		{Expect{RiskAtLeast: "MEDIUM"}, "CRITICAL", true},
		{Expect{RiskAtLeast: "MEDIUM"}, "LOW", false},
		{Expect{RiskAtLeast: "MEDIUM"}, "", false},
	} {
		if got := riskMatches(tc.e, tc.risk); got != tc.want {
			t.Errorf("%+v %q: %v", tc.e, tc.risk, got)
		}
	}
}

func TestGraderValidity(t *testing.T) {
	ok := Artifacts{State: "planned", Trace: "# Traceability · CHG-001 · complete\n", Blast: sampleBlast,
		Plan: "tasks:\n  - {id: T-01, kind: migrate}\n", Decisions: sampleDecisions, Review: sampleReview}
	g := GradeRun(phoneCase(), ok)
	if !g.Pass() || !g.ReviewOK {
		t.Errorf("valid run: %+v", g)
	}
	if g.Cards != 2 || g.CardsExpected != 1 || g.ExpectedRules != 2 || g.RulesFound != 1 || !slices.Contains(g.Problems, "no card for ask rule A2") {
		t.Errorf("cards: %+v", g)
	}
	for name, a := range map[string]Artifacts{
		"not planned":      {State: "awaiting_decisions", Trace: ok.Trace, Blast: ok.Blast, Plan: ok.Plan},
		"no state":         {Trace: ok.Trace, Blast: ok.Blast, Plan: ok.Plan},
		"incomplete trace": {State: "planned", Trace: "# Traceability · CHG-001 · incomplete\n", Blast: ok.Blast, Plan: ok.Plan},
		"no migrate task":  {State: "planned", Trace: ok.Trace, Blast: ok.Blast, Plan: "tasks:\n  - {id: T-01, kind: internal}\n"},
		"no plan":          {State: "planned", Trace: ok.Trace, Blast: ok.Blast},
	} {
		if GradeRun(phoneCase(), a).Pass() {
			t.Errorf("%s must fail EC-1", name)
		}
	}
}

func TestGraderReview(t *testing.T) {
	c := phoneCase()
	c.Expect.ReviewMissing = []string{"scope", "acceptance_criteria"}
	if g := GradeRun(c, Artifacts{Review: sampleReview}); g.ReviewOK || !slices.Contains(g.Problems, "the review marks acceptance_criteria present, want missing or partial") {
		t.Errorf("present is not missing: %+v", g)
	}
	if g := GradeRun(c, Artifacts{}); g.ReviewOK || !slices.Contains(g.Problems, "no PRD review") {
		t.Errorf("no review: %+v", g)
	}
}

func TestSummarise(t *testing.T) {
	cases := []Case{{ID: "a"}, {ID: "b"}}
	pass := Grade{Planned: true, TraceComplete: true, RiskOK: true, KindsOK: true, Expected: 10, Found: 10, ReportItems: 4, ReportMatched: 3,
		ExpectedRules: 1, RulesFound: 1, Cards: 2, CardsExpected: 1, ReviewOK: true}
	results := map[string][]Result{
		"a": {{Grade: pass, Pass: true, Sessions: []evalkit.Session{{CostUSD: 1, Duration: 10}, {CostUSD: 2, Duration: 5}}}},
		"b": {{Grade: pass, Pass: true}},
	}
	want := Summary{Runs: 2, Valid: 2, Recall: 1, Precision: 0.75, CardRecall: 1, CardPrecision: 0.5, ReviewOK: 2, CostUSD: 3, DurationS: 15, Met: true}
	if s := summarise(cases, results); s != want {
		t.Errorf("all valid: %+v, want %+v", s, want)
	}
	low := pass
	low.Found = 8
	results["b"] = []Result{{Grade: low, Pass: true}}
	if s := summarise(cases, results); s.Met || s.Recall != 0.9 {
		t.Errorf("recall 90%% misses EC-2: %+v", s)
	}
	results["b"] = []Result{{Grade: pass, Pass: false}}
	if s := summarise(cases, results); s.Met || s.Valid != 1 {
		t.Errorf("an invalid run misses EC-1: %+v", s)
	}
	if s := summarise(nil, nil); !s.Met || s.Recall != 1 {
		t.Errorf("nothing run: %+v", s)
	}
}
