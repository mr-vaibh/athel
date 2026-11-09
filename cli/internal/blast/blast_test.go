package blast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// baseline is a LOW report: one service, an additive change, a consumer with observed traffic.
const baseline = `schema: athel/blast@1
ask: ASK-002
mode: adhoc
proposal: Add an optional phone_verified field to GET /v1/users/{id}
services_changed: [users]
direct:
  - {item: "GET /v1/users/{id}", kind: interface, change: additive, service: users, evidence: [{kind: code, ref: users/api/user.go:40}]}
consumers:
  - {consumer: orders, of: "GET /v1/users/{id}", env: prod, in_change_set: false, traffic: {rate: 120, unit: calls_per_min, window: 7d, sampling: 1}, evidence: [{kind: trace, ref: "tempo:svc=orders"}], facts: []}
data: []
infra: []
people:
  - {service: users, owners: [identity], active_work: [], on_call: [ana]}
runtime:
  - {service: users, env: prod, calls_per_sec: 40, error_rate: 0.002, alerts: [], evidence: [{kind: metric, ref: "grafana:http_requests_total{service=users}"}]}
flags: {touches_auth_payments_pii: false, security_review_task: false, retry_timeout_or_idempotency_change: false}
not_affected:
  - {item: payments, why: no reads of users fields, evidence: [{kind: code, ref: "search:users.* in payments"}]}
gaps: []
risk: null
rules_fired: []
`

func rules(t *testing.T) Rules {
	t.Helper()
	r, err := LoadRules(config.Paths{Home: t.TempDir()}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func decode(t *testing.T, doc string) *Report {
	t.Helper()
	r, err := Decode(schema.NewRegistry(), []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBaselineIsLow(t *testing.T) {
	level, fired := Score(decode(t, baseline), rules(t))
	if level != "LOW" || len(fired) != 0 {
		t.Errorf("baseline = %s %+v", level, fired)
	}
}

func TestRiskRules(t *testing.T) {
	sub := func(pairs ...string) string {
		s := baseline
		for i := 0; i < len(pairs); i += 2 {
			if !strings.Contains(s, pairs[i]) {
				t.Fatalf("baseline lacks %q", pairs[i])
			}
			s = strings.Replace(s, pairs[i], pairs[i+1], 1)
		}
		return s
	}
	migration := func(op, rows string) string {
		return sub("data: []", "data:\n  - {table: users, service: users, operation: "+op+", rows: "+rows+", online: true, reversible: false, evidence: [{kind: migration, ref: m/001.sql}]}")
	}
	tests := []struct {
		name, doc, rule, level string
	}{
		{"breaking with a prod consumer outside the set", sub("change: additive", "change: breaking"), "breaking-consumer-outside-set", "CRITICAL"},
		{"destructive migration on a table with data", migration("drop", "10"), "destructive-migration-with-data", "CRITICAL"},
		{"destructive migration of unknown size", migration("rename", "null"), "destructive-migration-with-data", "CRITICAL"},
		{"sensitive without security review", sub("touches_auth_payments_pii: false", "touches_auth_payments_pii: true"), "sensitive-without-security-review", "CRITICAL"},
		{"firing critical alert", sub("alerts: []", "alerts: [{name: HighErrorRate, severity: critical, firing: true}]"), "critical-alert-firing", "CRITICAL"},
		{"breaking, every consumer in the set", sub("change: additive", "change: breaking", "in_change_set: false", "in_change_set: true"), "breaking-all-in-set", "HIGH"},
		{"migration on a large table", migration("add", "2100000"), "large-table-migration", "HIGH"},
		{"more than three services", sub("services_changed: [users]", "services_changed: [users, orders, payments, gateway]"), "many-services", "HIGH"},
		{"new external dependency", sub("infra: []", "infra: [{item: twilio, kind: external_dependency, new: true, evidence: [{kind: code, ref: go.mod}]}]"), "new-external-dependency", "HIGH"},
		{"retry change on a hot path", sub("retry_timeout_or_idempotency_change: false", "retry_timeout_or_idempotency_change: true", "calls_per_sec: 40", "calls_per_sec: 250"), "hot-path-behaviour-change", "HIGH"},
		{"retry change, traffic unknown", sub("retry_timeout_or_idempotency_change: false", "retry_timeout_or_idempotency_change: true", "calls_per_sec: 40", "calls_per_sec: null"), "hot-path-behaviour-change", "HIGH"},
		{"behavioural change with consumers", sub("change: additive", "change: behavioural"), "behavioural-with-consumers", "MEDIUM"},
		{"any migration", migration("add", "10"), "any-migration", "MEDIUM"},
		{"new secret", sub("infra: []", "infra: [{item: SMS_KEY, kind: secret, new: true, evidence: [{kind: manifest, ref: k8s/users.yaml}]}]"), "new-config-or-secret", "MEDIUM"},
		{"two services", sub("services_changed: [users]", "services_changed: [users, orders]"), "several-services", "MEDIUM"},
		{"consumer without traffic", sub("traffic: {rate: 120, unit: calls_per_min, window: 7d, sampling: 1}", "traffic: null"), "no-runtime-evidence", "MEDIUM"},
		{"no runtime section", sub("runtime:\n  - {service: users, env: prod, calls_per_sec: 40, error_rate: 0.002, alerts: [], evidence: [{kind: metric, ref: \"grafana:http_requests_total{service=users}\"}]}", "runtime: []"), "no-runtime-evidence", "MEDIUM"},
	}
	r := rules(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, fired := Score(decode(t, tt.doc), r)
			found := false
			for _, f := range fired {
				found = found || (f.Rule == tt.rule && f.Level == tt.level && f.Reason != "")
			}
			if !found || level != tt.level && !higher(level, tt.level) {
				t.Errorf("level %s fired %+v; want rule %s at %s", level, fired, tt.rule, tt.level)
			}
		})
	}
}

func higher(a, b string) bool {
	ia, ib := -1, -1
	for i, l := range Levels {
		if l == a {
			ia = i
		}
		if l == b {
			ib = i
		}
	}
	return ia > ib
}

func TestHighestLevelWinsAndOrderIsStable(t *testing.T) {
	doc := strings.Replace(baseline, "services_changed: [users]", "services_changed: [users, orders]", 1)
	doc = strings.Replace(doc, "change: additive", "change: breaking", 1)
	r := rules(t)
	level, fired := Score(decode(t, doc), r)
	if level != "CRITICAL" || len(fired) != 2 || fired[0].Rule != "breaking-consumer-outside-set" || fired[1].Rule != "several-services" {
		t.Fatalf("level %s fired %+v", level, fired)
	}
	for range 20 {
		if l, f := Score(decode(t, doc), r); l != level || len(f) != len(fired) || f[0] != fired[0] {
			t.Fatal("the same input gave a different result")
		}
	}
}

func TestStagingConsumerIsNotProduction(t *testing.T) {
	doc := strings.Replace(strings.Replace(baseline, "change: additive", "change: breaking", 1), "env: prod, in_change_set", "env: staging, in_change_set", 1)
	if level, fired := Score(decode(t, doc), rules(t)); level != "HIGH" || fired[0].Rule != "breaking-all-in-set" {
		t.Errorf("breaking change with only a staging consumer = %s %+v", level, fired)
	}
}

func TestDecodeErrors(t *testing.T) {
	reg := schema.NewRegistry()
	for name, doc := range map[string]string{
		"no not_affected list":      strings.Replace(baseline, "not_affected:\n  - {item: payments, why: no reads of users fields, evidence: [{kind: code, ref: \"search:users.* in payments\"}]}\n", "", 1),
		"consumer without evidence": strings.Replace(baseline, `evidence: [{kind: trace, ref: "tempo:svc=orders"}]`, "evidence: []", 1),
		"unknown field":             baseline + "extra: 1\n",
		"model-chosen risk":         strings.Replace(baseline, "risk: null", "risk: NEGLIGIBLE", 1),
		"empty":                     "",
	} {
		if _, err := Decode(reg, []byte(doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRulesTightenOnly(t *testing.T) {
	reg := schema.NewRegistry()
	home := t.TempDir()
	org := filepath.Join(home, "org", "blast.yaml")
	if err := os.MkdirAll(filepath.Dir(org), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(org, []byte("schema: athel/blast-rules@1\nthresholds: {large_table_rows: 100000}\nlevels: {any-migration: HIGH}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRules(config.Paths{Home: home}, reg)
	if err != nil || r.LargeTableRows != 100000 || r.Levels["any-migration"] != "HIGH" || r.ManyServices != 3 || r.HotPathCallsPerSec != 100 {
		t.Fatalf("rules = %+v %v", r, err)
	}
	if err := os.WriteFile(org, []byte("schema: athel/blast-rules@1\nlevels: {critical-alert-firing: LOW}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(config.Paths{Home: home}, reg); !aerr.HasCode(err, aerr.PolicyConfigLoosens) {
		t.Errorf("lowering a level: %v", err)
	}
	if number("x") != 0 || number(2.5) != 2.5 {
		t.Error("number")
	}
}

func TestRender(t *testing.T) {
	doc := strings.Replace(baseline, "change: additive", "change: breaking", 1)
	doc = strings.Replace(doc, "data: []", "data:\n  - {table: users, service: users, operation: add, rows: null, online: true, reversible: true, evidence: [{kind: migration, ref: m/1.sql}]}\n  - {table: users, service: users, operation: index, rows: 5, online: true, reversible: true, evidence: [{kind: migration, ref: m/2.sql}]}", 1)
	doc = strings.Replace(doc, "infra: []", "infra: [{item: SMS_KEY, kind: secret, new: true, evidence: [{kind: manifest, ref: k8s/users.yaml}]}]", 1)
	doc = strings.Replace(doc, "active_work: []", "active_work: [\"users#812 touches src/profile | api\"]", 1)
	doc = strings.Replace(doc, "sampling: 1}", "sampling: 0.01}", 1)
	doc = strings.Replace(doc, "gaps: []", "gaps: [coralogix not authenticated]", 1)
	r := decode(t, doc)
	level, fired := Score(r, rules(t))
	r.Risk, r.RulesFired = &level, fired
	md := string(Render(r, "Three things matter."))
	for _, want := range []string{"# Blast radius · ASK-002 · adhoc · risk CRITICAL", "Rules fired: breaking-consumer-outside-set (CRITICAL:",
		"| Consumer | orders uses GET /v1/users/{id} | trace tempo:svc=orders | 120 calls per min (prod, 7d), sampled at 0.01 |",
		"| Data | add users.users | migration m/1.sql | unknown size |", "5 rows", "| Infra | secret: SMS_KEY |",
		`users#812 touches src/profile \| api`, "Not affected (checked):\n- payments:", "Gaps: coralogix not authenticated",
		"Three things matter.", "```yaml\nschema: athel/blast@1"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q:\n%s", want, md)
		}
	}
	plain := decode(t, strings.Replace(strings.Replace(baseline, "not_affected:\n  - {item: payments, why: no reads of users fields, evidence: [{kind: code, ref: \"search:users.* in payments\"}]}", "not_affected: []", 1), "traffic: {rate: 120, unit: calls_per_min, window: 7d, sampling: 1}", "traffic: null", 1))
	md = string(Render(plain, ""))
	for _, want := range []string{"risk unscored", "Rules fired: none", "- nothing was ruled out", "not observed"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q", want)
		}
	}
}
