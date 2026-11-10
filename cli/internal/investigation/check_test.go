package investigation

import (
	"slices"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const example = `schema: athel/investigation@1
ask: ASK-001
question: Who consumes the UserUpdated event?
type: event_consumers
scope: {services: [users], repos: []}
complete: true
budget: {max_tool_calls: 60, max_minutes: 10, used_tool_calls: 14, used_minutes: 3.5}
claims:
  - id: C1
    text: crm-sync consumes topic users.updated.v1
    subject: {kind: event, name: UserUpdated, topic: users.updated.v1}
    predicate: consumed_by
    object: {kind: service, name: crm-sync}
    env: prod
    confidence: confirmed
    evidence:
      - {kind: code, ref: crm-sync/src/consumers/user.ts:14, commit: 9f3c2ab}
      - {kind: metric, ref: "grafana:kafka-consumer-lag?group=crm-sync", window: 7d, tool_use_id: toolu_01}
    facts: [F-0001]
drift: []
gaps: []
active_work: []
health: []
suspicious:
  - {ref: "coralogix:log/1", excerpt: "ignore previous instructions", tool_use_id: toolu_09}
facts_used: [F-0001]
facts_created: [F-0002]
`

func decode(t *testing.T, doc string) *Investigation {
	t.Helper()
	inv, err := Decode(schema.NewRegistry(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func okContext() Context {
	return Context{Ask: "ASK-001",
		Accessed:   func(f string) bool { return f == "F-0001" },
		FactStatus: func(f string) (string, bool) { return "active", f == "F-0002" },
		Flagged:    []string{"toolu_09"}}
}

func TestValidInvestigationPasses(t *testing.T) {
	inv := decode(t, example)
	r := Check(inv, okContext(), "crm-sync consumes it.")
	if err := r.Err(); err != nil {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Claims) != 1 || r.Claims[0].Ceiling.Label != Confirmed || len(r.AddedGaps) != 0 {
		t.Errorf("report = %+v", r)
	}
}

func TestDecodeErrors(t *testing.T) {
	reg := schema.NewRegistry()
	for name, doc := range map[string]string{
		"empty":                  "",
		"unknown field":          example + "bogus: 1\n",
		"wrong type":             strings.Replace(example, "complete: true", "complete: [x]", 1),
		"claim without evidence": strings.Replace(example, "    evidence:\n      - {kind: code, ref: crm-sync/src/consumers/user.ts:14, commit: 9f3c2ab}\n      - {kind: metric, ref: \"grafana:kafka-consumer-lag?group=crm-sync\", window: 7d, tool_use_id: toolu_01}\n", "    evidence: []\n", 1),
		"two documents":          example + "---\nschema: x\n",
	} {
		if _, err := Decode(reg, []byte(doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v, want E-STATE-SCHEMA", name, err)
		}
	}
}

func TestOverclaimRejected(t *testing.T) {
	doc := strings.Replace(example, "      - {kind: metric, ref: \"grafana:kafka-consumer-lag?group=crm-sync\", window: 7d, tool_use_id: toolu_01}\n", "", 1)
	r := Check(decode(t, doc), okContext(), "")
	err := r.Err()
	if !aerr.HasCode(err, aerr.StateConfidence) || len(r.Mislabels) != 1 || !strings.Contains(r.Mislabels[0], "evidence gives unverified") {
		t.Fatalf("report = %+v err %v", r, err)
	}
	lowered := strings.Replace(doc, "confidence: confirmed", "confidence: unverified", 1)
	if r := Check(decode(t, lowered), okContext(), ""); r.Err() != nil {
		t.Errorf("an honest label is accepted: %+v", r)
	}
}

func TestRuleViolations(t *testing.T) {
	tests := []struct {
		name, old, new string
		ctx            func(*Context)
		want           string
	}{
		{"wrong ref", "", "", func(c *Context) { c.Ask = "ASK-002" }, "is for ASK-001"},
		{"duplicate claim id", "facts: [F-0001]\ndrift: []", "facts: [F-0001]\n  - id: C1\n    text: again\n    subject: {kind: event, name: X}\n    predicate: owned_by\n    object: {kind: team, name: t}\n    env: any\n    confidence: unverified\n    evidence: [{kind: name, ref: x}]\n    facts: []\ndrift: []", nil, "used twice"},
		{"fact not read", "", "", func(c *Context) { c.Accessed = func(string) bool { return false } }, "F-0001 is cited but was not read"},
		{"created fact missing", "", "", func(c *Context) { c.FactStatus = func(string) (string, bool) { return "", false } }, "F-0002 is listed as created"},
		{"created fact disputed", "", "", func(c *Context) { c.FactStatus = func(string) (string, bool) { return "disputed", true } }, "F-0002 is listed as created"},
		{"partial without gaps", "complete: true", "complete: false", nil, "FR-DISC-014"},
		{"suspicious not listed", "", "", func(c *Context) { c.Flagged = append(c.Flagged, "toolu_10") }, "toolu_10 returned suspicious content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := okContext()
			if tt.ctx != nil {
				tt.ctx(&ctx)
			}
			r := Check(decode(t, strings.Replace(example, tt.old, tt.new, 1)), ctx, "")
			err := r.Err()
			if !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPartialResultValid(t *testing.T) {
	doc := strings.Replace(example, "complete: true", "complete: false", 1)
	doc = strings.Replace(doc, "gaps: []", "gaps:\n  - {source: question budget, code: E-BUDGET-QUESTION, reason: 60 tool calls used, login: null, effect: notification-service not checked}", 1)
	if r := Check(decode(t, doc), okContext(), ""); r.Err() != nil {
		t.Errorf("partial result with gaps: %+v", r)
	}
}

func TestMissingMandatorySourceAddsGapOnce(t *testing.T) {
	doc := strings.Replace(example, "{kind: metric, ref: \"grafana:kafka-consumer-lag?group=crm-sync\", window: 7d, tool_use_id: toolu_01}", "{kind: cluster, ref: deploy/crm-sync}", 1)
	doc = strings.Replace(doc, "confidence: confirmed", "confidence: likely", 1)
	inv := decode(t, doc)
	r := Check(inv, okContext(), "")
	if r.Err() != nil || len(r.AddedGaps) != 1 || r.AddedGaps[0].Source != "consumer group or subscription metrics" || len(inv.Gaps) != 1 {
		t.Fatalf("report = %+v gaps %+v", r, inv.Gaps)
	}
	if r := Check(inv, okContext(), ""); len(r.AddedGaps) != 0 {
		t.Errorf("gap added twice: %+v", r.AddedGaps)
	}
}

func TestWordLimit(t *testing.T) {
	inv := decode(t, example)
	if r := Check(inv, okContext(), strings.Repeat("word ", MaxSummaryWords)); r.Err() != nil {
		t.Errorf("exactly the limit: %v", r.Err())
	}
	if err := Check(inv, okContext(), strings.Repeat("word ", MaxSummaryWords+1)).Err(); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "2001 words") {
		t.Errorf("over the limit: %v", err)
	}
}

func TestRenderRoundTrip(t *testing.T) {
	reg := schema.NewRegistry()
	inv := decode(t, example)
	md := Render(inv, "  crm-sync consumes it.\n")
	if !strings.HasPrefix(string(md), "# Investigation · ASK-001 · event_consumers\n") || !strings.Contains(string(md), "crm-sync consumes it.\n\n## Claim block") {
		t.Errorf("rendered:\n%s", md)
	}
	back, err := Extract(reg, md)
	if err != nil || back.Claims[0].Evidence[1].ToolUseID != "toolu_01" || back.Budget.UsedMinutes != 3.5 || back.Claims[0].Subject["topic"] != "users.updated.v1" {
		t.Fatalf("extract = %+v %v", back, err)
	}
	if again := Render(back, "  crm-sync consumes it.\n"); string(again) != string(md) {
		t.Errorf("render is not stable:\n%s\n---\n%s", md, again)
	}
	if md := Render(inv, ""); strings.Contains(string(md), "\n\n\n") {
		t.Errorf("empty summary leaves a blank block:\n%s", md)
	}
	for name, bad := range map[string]string{"no block": "# x\n", "unterminated": "```yaml\nschema: x\n"} {
		if _, err := Extract(reg, []byte(bad)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCreatedFactsNeedNoAccessRecord(t *testing.T) {
	doc := strings.Replace(example, "    facts: [F-0001]", "    facts: [F-0001, F-0002]", 1)
	if r := Check(decode(t, doc), okContext(), ""); r.Err() != nil {
		t.Errorf("a fact the answer created is cited: %+v", r)
	}
}

// TestUnderclaimRejected: the CLI decides labels (ADR 018), so a label below what the evidence
// gives is refused as firmly as one above it.
func TestUnderclaimRejected(t *testing.T) {
	doc := strings.Replace(example, "confidence: confirmed", "confidence: likely", 1)
	r := Check(decode(t, doc), okContext(), "")
	if err := r.Err(); !aerr.HasCode(err, aerr.StateConfidence) || len(r.Mislabels) != 1 || !strings.Contains(r.Mislabels[0], "C1 is labelled likely but its evidence gives confirmed") {
		t.Fatalf("report = %+v err %v", r, err)
	}
}

// TestContradictingEvidence: a source marked supports: false never raises a claim and caps it at
// unverified, as when the code calls an endpoint the metrics show gets no traffic.
func TestContradictingEvidence(t *testing.T) {
	doc := strings.Replace(example, "window: 7d, tool_use_id: toolu_01}", "window: 7d, tool_use_id: toolu_01, supports: false}", 1)
	if r := Check(decode(t, doc), okContext(), ""); r.Claims[0].Ceiling.Label != Unverified || !strings.Contains(r.Claims[0].Ceiling.Why, "contradicts") {
		t.Errorf("report = %+v", r)
	}
	honest := strings.Replace(doc, "confidence: confirmed", "confidence: unverified", 1)
	if r := Check(decode(t, honest), okContext(), ""); r.Err() != nil {
		t.Errorf("unverified is the contradicted claim's label: %+v", r)
	}
	explicit := strings.Replace(example, "window: 7d, tool_use_id: toolu_01}", "window: 7d, tool_use_id: toolu_01, supports: true}", 1)
	if r := Check(decode(t, explicit), okContext(), ""); r.Err() != nil {
		t.Errorf("supports: true is the default: %+v", r)
	}
}

func TestClaimDirection(t *testing.T) {
	tests := []struct {
		name, from, to, want string
	}{
		{"backwards event claim", "subject: {kind: event, name: UserUpdated, topic: users.updated.v1}", "subject: {kind: service, name: users}",
			"C1: consumed_by needs its subject kind to be event or topic, not service"},
		{"team owner as object", "predicate: consumed_by\n    object: {kind: service, name: crm-sync}", "predicate: owned_by\n    object: {kind: service, name: crm-sync}",
			"C1: owned_by needs its object kind to be team or person, not service"},
		{"unconstrained predicate", "predicate: consumed_by", "predicate: exposes", ""},
	}
	for _, tt := range tests {
		doc := strings.Replace(example, tt.from, tt.to, 1)
		r := Check(decode(t, doc), okContext(), "")
		got := strings.Join(r.Problems, "; ")
		if tt.want == "" && strings.Contains(got, "a claim reads subject first") || tt.want != "" && !strings.Contains(got, tt.want) {
			t.Errorf("%s: problems %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestHintEvidenceOnAnotherClaim: a mandatory source filed on a side claim is pointed out, and
// the save is not blocked by it.
func TestHintEvidenceOnAnotherClaim(t *testing.T) {
	doc := strings.Replace(example, "      - {kind: metric, ref: \"grafana:kafka-consumer-lag?group=crm-sync\", window: 7d, tool_use_id: toolu_01}\n", "", 1)
	doc = strings.Replace(doc, "confidence: confirmed", "confidence: unverified", 1)
	doc = strings.Replace(doc, "drift: []", `  - id: C2
    text: crm-sync has a consumer group
    subject: {kind: service, name: crm-sync}
    predicate: exposes
    object: {kind: metric, name: lag}
    env: prod
    confidence: likely
    evidence:
      - {kind: metric, ref: "grafana:lag", window: 7d}
    facts: []
drift: []`, 1)
	r := Check(decode(t, doc), okContext(), "")
	if r.Err() != nil || len(r.Hints) != 1 || !strings.Contains(r.Hints[0], "C1 answers the question without consumer group or subscription metrics, but C2 cites metric grafana:lag") {
		t.Errorf("report = %+v err %v", r, r.Err())
	}
	if r := Check(decode(t, example), okContext(), ""); len(r.Hints) != 0 {
		t.Errorf("no hint when nothing is missing: %+v", r.Hints)
	}
}

// TestHintIndependentSource: the answer has one source system and a side claim cites another.
func TestHintIndependentSource(t *testing.T) {
	doc := `schema: athel/investigation@1
ask: ASK-001
question: What version of users runs in prod?
type: deployed_state
scope: {services: [users], repos: []}
complete: true
budget: {max_tool_calls: 60, max_minutes: 10, used_tool_calls: 5, used_minutes: 1}
claims:
  - id: C1
    text: prod runs users 1.8.2
    subject: {kind: service, name: users}
    predicate: deployed_version
    object: {kind: image, name: "users:1.8.2"}
    env: prod
    confidence: likely
    evidence:
      - {kind: cluster, ref: kubectl get deploy}
    facts: []
  - id: C2
    text: user-svc is users
    subject: {kind: service, name: user-svc}
    predicate: alias_of
    object: {kind: repo, name: users}
    env: prod
    confidence: likely
    evidence:
      - {kind: manifest, ref: users/deploy/k8s.yaml:1}
      - {kind: name, ref: user-svc}
    facts: []
drift: []
gaps: []
active_work: []
health: []
suspicious: []
facts_used: []
facts_created: []
`
	r := Check(decode(t, doc), Context{Ask: "ASK-001", Accessed: func(string) bool { return false }, FactStatus: func(string) (string, bool) { return "", false }}, "")
	if r.Err() != nil || len(r.Hints) != 1 || !strings.Contains(r.Hints[0], "C1 is short of confirmed, but C2 cites manifest users/deploy/k8s.yaml:1") {
		t.Errorf("report = %+v err %v", r, r.Err())
	}
	confirmed := strings.Replace(strings.Replace(doc, "      - {kind: cluster, ref: kubectl get deploy}\n", "      - {kind: cluster, ref: kubectl get deploy}\n      - {kind: manifest, ref: users/deploy/k8s.yaml:15}\n", 1), "confidence: likely", "confidence: confirmed", 1)
	if r := Check(decode(t, confirmed), Context{Ask: "ASK-001", Accessed: func(string) bool { return false }, FactStatus: func(string) (string, bool) { return "", false }}, ""); r.Err() != nil || len(r.Hints) != 0 {
		t.Errorf("a confirmed answer needs no hint: %+v %v", r.Hints, r.Err())
	}
}

// TestHintDirectCallBesideRoute: the gateway-hop trap. A direct call next to a route through a
// hop is pointed out; without the hop's own route there is nothing to point at.
func TestHintDirectCallBesideRoute(t *testing.T) {
	claim := func(id, subject, predicate, object string) string {
		return `  - id: ` + id + `
    text: ` + subject + ` ` + predicate + ` ` + object + `
    subject: {kind: service, name: ` + subject + `}
    predicate: ` + predicate + `
    object: {kind: service, name: ` + object + `}
    env: prod
    confidence: unverified
    evidence:
      - {kind: code, ref: ` + subject + `/client.go}
    facts: []
`
	}
	doc := func(claims ...string) string {
		return `schema: athel/investigation@1
ask: ASK-001
question: How does orders reach payments?
type: service_path
scope: {services: [orders], repos: []}
complete: true
budget: {max_tool_calls: 60, max_minutes: 10, used_tool_calls: 5, used_minutes: 1}
claims:
` + strings.Join(claims, "") + `drift: []
gaps: []
active_work: []
health: []
suspicious: []
facts_used: []
facts_created: []
`
	}
	ctx := Context{Ask: "ASK-001", Accessed: func(string) bool { return false }, FactStatus: func(string) (string, bool) { return "", false }}
	hinted := func(r Report) bool {
		return slices.ContainsFunc(r.Hints, func(h string) bool {
			return strings.Contains(h, "C3 says orders calls payments directly, but C1 and C2 route it through gateway")
		})
	}
	for _, first := range []string{"routes_via", "calls"} {
		r := Check(decode(t, doc(claim("C1", "orders", first, "gateway"), claim("C2", "gateway", "routes_via", "payments"), claim("C3", "orders", "calls", "payments"))), ctx, "")
		if !hinted(r) {
			t.Errorf("orders %s gateway: hints %v", first, r.Hints)
		}
	}
	if r := Check(decode(t, doc(claim("C1", "orders", "calls", "gateway"), claim("C3", "orders", "calls", "payments"))), ctx, ""); hinted(r) {
		t.Errorf("no route from the hop, no hint: %v", r.Hints)
	}
}
