// Package blast computes the risk of a blast radius report with doc 05's rules (§Risk rules):
// deterministic, the highest level that fires wins, every rule that fired is listed
// (FR-BLAST-004). The model writes the findings; it never sets the risk.
package blast

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Schemas.
const (
	SchemaID      = "athel/blast@1"
	RulesSchemaID = "athel/blast-rules@1"
)

// Levels are the risk levels, weakest first.
var Levels = []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}

// Report is one blast@1 document. Field order is the YAML output order.
type Report struct {
	Schema          string        `yaml:"schema" json:"schema"`
	Change          string        `yaml:"change,omitempty" json:"change,omitempty"`
	Ask             string        `yaml:"ask,omitempty" json:"ask,omitempty"`
	Mode            string        `yaml:"mode" json:"mode"`
	Proposal        string        `yaml:"proposal" json:"proposal"`
	ServicesChanged []string      `yaml:"services_changed" json:"services_changed"`
	Direct          []Direct      `yaml:"direct" json:"direct"`
	Consumers       []Consumer    `yaml:"consumers" json:"consumers"`
	Data            []Migration   `yaml:"data" json:"data"`
	Infra           []Infra       `yaml:"infra" json:"infra"`
	People          []People      `yaml:"people" json:"people"`
	Runtime         []Runtime     `yaml:"runtime" json:"runtime"`
	Flags           Flags         `yaml:"flags" json:"flags"`
	NotAffected     []NotAffected `yaml:"not_affected" json:"not_affected"`
	Gaps            []string      `yaml:"gaps" json:"gaps"`
	Risk            *string       `yaml:"risk" json:"risk"`
	RulesFired      []Fired       `yaml:"rules_fired" json:"rules_fired"`
}

// Evidence is one source reference.
type Evidence struct {
	Kind string `yaml:"kind" json:"kind"`
	Ref  string `yaml:"ref" json:"ref"`
}

// Direct is one changed interface, event, schema, config, code path or dependency.
type Direct struct {
	Item     string     `yaml:"item" json:"item"`
	Kind     string     `yaml:"kind" json:"kind"`
	Change   string     `yaml:"change" json:"change"`
	Service  string     `yaml:"service" json:"service"`
	Evidence []Evidence `yaml:"evidence" json:"evidence"`
}

// Traffic is observed use of a contract by one consumer.
type Traffic struct {
	Rate     float64 `yaml:"rate" json:"rate"`
	Unit     string  `yaml:"unit" json:"unit"`
	Window   string  `yaml:"window" json:"window"`
	Sampling float64 `yaml:"sampling" json:"sampling"`
}

// Consumer uses a changed contract.
type Consumer struct {
	Consumer    string     `yaml:"consumer" json:"consumer"`
	Of          string     `yaml:"of" json:"of"`
	Env         string     `yaml:"env" json:"env"`
	InChangeSet bool       `yaml:"in_change_set" json:"in_change_set"`
	Traffic     *Traffic   `yaml:"traffic" json:"traffic"`
	Evidence    []Evidence `yaml:"evidence" json:"evidence"`
	Facts       []string   `yaml:"facts" json:"facts"`
}

// Migration is one data change.
type Migration struct {
	Table      string     `yaml:"table" json:"table"`
	Service    string     `yaml:"service" json:"service"`
	Operation  string     `yaml:"operation" json:"operation"`
	Rows       *int64     `yaml:"rows" json:"rows"`
	Online     bool       `yaml:"online" json:"online"`
	Reversible bool       `yaml:"reversible" json:"reversible"`
	Evidence   []Evidence `yaml:"evidence" json:"evidence"`
}

// Infra is one infrastructure or config change.
type Infra struct {
	Item     string     `yaml:"item" json:"item"`
	Kind     string     `yaml:"kind" json:"kind"`
	New      bool       `yaml:"new" json:"new"`
	Evidence []Evidence `yaml:"evidence" json:"evidence"`
}

// People are the owners, overlapping work and on-call of one affected service.
type People struct {
	Service    string   `yaml:"service" json:"service"`
	Owners     []string `yaml:"owners" json:"owners"`
	ActiveWork []string `yaml:"active_work" json:"active_work"`
	OnCall     []string `yaml:"on_call" json:"on_call"`
}

// Alert is one alert on a service.
type Alert struct {
	Name     string `yaml:"name" json:"name"`
	Severity string `yaml:"severity" json:"severity"`
	Firing   bool   `yaml:"firing" json:"firing"`
}

// Runtime is the current health of one affected service.
type Runtime struct {
	Service     string     `yaml:"service" json:"service"`
	Env         string     `yaml:"env" json:"env"`
	CallsPerSec *float64   `yaml:"calls_per_sec" json:"calls_per_sec"`
	ErrorRate   *float64   `yaml:"error_rate" json:"error_rate"`
	Alerts      []Alert    `yaml:"alerts" json:"alerts"`
	Evidence    []Evidence `yaml:"evidence" json:"evidence"`
}

// Flags are facts about the change the rules need.
type Flags struct {
	TouchesAuthPaymentsPII  bool `yaml:"touches_auth_payments_pii" json:"touches_auth_payments_pii"`
	SecurityReviewTask      bool `yaml:"security_review_task" json:"security_review_task"`
	RetryTimeoutIdempotency bool `yaml:"retry_timeout_or_idempotency_change" json:"retry_timeout_or_idempotency_change"`
}

// NotAffected is something ruled out, with how.
type NotAffected struct {
	Item     string     `yaml:"item" json:"item"`
	Why      string     `yaml:"why" json:"why"`
	Evidence []Evidence `yaml:"evidence" json:"evidence"`
}

// Fired is one rule that fired.
type Fired struct {
	Rule   string `yaml:"rule" json:"rule"`
	Level  string `yaml:"level" json:"level"`
	Reason string `yaml:"reason" json:"reason"`
}

// Decode reads one blast@1 document: a strict decode, then the schema.
func Decode(reg *schema.Registry, data []byte) (*Report, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var r Report
	if err := dec.Decode(&r); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode blast radius report", err)
	}
	if err := reg.ValidateYAML(SchemaID, data); err != nil {
		return nil, err
	}
	return &r, nil
}

// Rules are the thresholds and levels from blast.yaml.
type Rules struct {
	LargeTableRows     int64
	HotPathCallsPerSec float64
	ManyServices       int
	Levels             map[string]string
}

// Spec is blast.yaml as a layered policy file (org and project layers may only tighten).
var Spec = config.FileSpec{
	Name: "blast.yaml", Schema: RulesSchemaID,
	Rules: config.Rules{
		"schema":                            {Kind: config.Free},
		"thresholds.large_table_rows":       {Kind: config.Min},
		"thresholds.hot_path_calls_per_sec": {Kind: config.Min},
		"thresholds.many_services":          {Kind: config.Min},
		"levels.*":                          {Kind: config.Ordered, Order: Levels},
	},
}

// LoadRules reads the effective blast.yaml.
func LoadRules(p config.Paths, reg *schema.Registry) (Rules, error) {
	eff, _, err := config.LoadFile(p, reg, Spec)
	if err != nil {
		return Rules{}, err
	}
	th, _ := eff.Values["thresholds"].(map[string]any)
	r := Rules{LargeTableRows: int64(number(th["large_table_rows"])), HotPathCallsPerSec: number(th["hot_path_calls_per_sec"]),
		ManyServices: int(number(th["many_services"])), Levels: map[string]string{}}
	levels, _ := eff.Values["levels"].(map[string]any)
	for k, v := range levels {
		r.Levels[k], _ = v.(string)
	}
	return r, nil
}

func number(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// rule is one of doc 05's risk rules: it fires with a reason, or returns "".
type rule struct {
	name string
	test func(r *Report, rules Rules) string
}

// order is doc 05's table, top to bottom; reports list fired rules in this order.
var order = []rule{
	{"breaking-consumer-outside-set", breakingOutsideSet},
	{"destructive-migration-with-data", destructiveMigration},
	{"sensitive-without-security-review", sensitiveWithoutReview},
	{"critical-alert-firing", criticalAlert},
	{"breaking-all-in-set", breakingAllInSet},
	{"large-table-migration", largeTable},
	{"many-services", manyServices},
	{"new-external-dependency", func(r *Report, _ Rules) string { return infraNew(r, "external_dependency") }},
	{"hot-path-behaviour-change", hotPath},
	{"behavioural-with-consumers", behaviouralWithConsumers},
	{"any-migration", anyMigration},
	{"new-config-or-secret", func(r *Report, _ Rules) string { return infraNew(r, "env_var", "secret") }},
	{"several-services", severalServices},
	{"no-runtime-evidence", noRuntimeEvidence},
}

// Score applies every rule; the risk is the highest level fired, LOW when none fires.
func Score(r *Report, rules Rules) (string, []Fired) {
	fired := []Fired{}
	level := 0
	for _, ru := range order {
		reason := ru.test(r, rules)
		if reason == "" {
			continue
		}
		lv := rules.Levels[ru.name]
		fired = append(fired, Fired{Rule: ru.name, Level: lv, Reason: reason})
		level = max(level, slices.Index(Levels, lv))
	}
	return Levels[level], fired
}

func prodLike(env string) bool { return env == "prod" || env == "any" }

func consumersOf(r *Report, item string) []Consumer {
	var out []Consumer
	for _, c := range r.Consumers {
		if c.Of == item {
			out = append(out, c)
		}
	}
	return out
}

func breakingOutsideSet(r *Report, _ Rules) string {
	for _, d := range r.Direct {
		if d.Change != "breaking" {
			continue
		}
		for _, c := range consumersOf(r, d.Item) {
			if !c.InChangeSet && prodLike(c.Env) {
				return fmt.Sprintf("breaking change to %s, consumed in %s by %s outside the change set", d.Item, c.Env, c.Consumer)
			}
		}
	}
	return ""
}

func breakingAllInSet(r *Report, rules Rules) string {
	if breakingOutsideSet(r, rules) != "" {
		return ""
	}
	for _, d := range r.Direct {
		if d.Change == "breaking" {
			return "breaking change to " + d.Item + "; every known consumer is in the change set"
		}
	}
	return ""
}

func destructiveMigration(r *Report, _ Rules) string {
	for _, m := range r.Data {
		if slices.Contains([]string{"drop", "rename", "narrow"}, m.Operation) && (m.Rows == nil || *m.Rows > 0) {
			return fmt.Sprintf("%s on %s, a table with data", m.Operation, m.Table)
		}
	}
	return ""
}

func sensitiveWithoutReview(r *Report, _ Rules) string {
	if r.Flags.TouchesAuthPaymentsPII && !r.Flags.SecurityReviewTask {
		return "touches auth, payments or PII handling without a security review task"
	}
	return ""
}

func criticalAlert(r *Report, _ Rules) string {
	for _, rt := range r.Runtime {
		for _, a := range rt.Alerts {
			if a.Severity == "critical" && a.Firing {
				return fmt.Sprintf("%s has a firing critical alert (%s) in %s", rt.Service, a.Name, rt.Env)
			}
		}
	}
	return ""
}

// largeTable treats an unknown row count as large: the safe reading.
func largeTable(r *Report, rules Rules) string {
	for _, m := range r.Data {
		if m.Rows == nil {
			return "migration on " + m.Table + ", whose size is unknown"
		}
		if *m.Rows > rules.LargeTableRows {
			return fmt.Sprintf("migration on %s with %d rows (over %d)", m.Table, *m.Rows, rules.LargeTableRows)
		}
	}
	return ""
}

func manyServices(r *Report, rules Rules) string {
	if n := len(r.ServicesChanged); n > rules.ManyServices {
		return fmt.Sprintf("%d services changed (more than %d)", n, rules.ManyServices)
	}
	return ""
}

func severalServices(r *Report, rules Rules) string {
	if n := len(r.ServicesChanged); n >= 2 && n <= rules.ManyServices {
		return fmt.Sprintf("%d services changed: %s", n, strings.Join(r.ServicesChanged, ", "))
	}
	return ""
}

func infraNew(r *Report, kinds ...string) string {
	for _, i := range r.Infra {
		if i.New && slices.Contains(kinds, i.Kind) {
			return "new " + strings.ReplaceAll(i.Kind, "_", " ") + ": " + i.Item
		}
	}
	return ""
}

// hotPath treats a changed service with unknown traffic as hot: the safe reading.
func hotPath(r *Report, rules Rules) string {
	if !r.Flags.RetryTimeoutIdempotency {
		return ""
	}
	for _, rt := range r.Runtime {
		if !slices.Contains(r.ServicesChanged, rt.Service) {
			continue
		}
		if rt.CallsPerSec == nil || *rt.CallsPerSec > rules.HotPathCallsPerSec {
			return "retry, timeout or idempotency change on " + rt.Service + ", a hot path or one of unknown traffic"
		}
	}
	return ""
}

func behaviouralWithConsumers(r *Report, _ Rules) string {
	for _, d := range r.Direct {
		if d.Change == "behavioural" && len(consumersOf(r, d.Item)) > 0 {
			return "behavioural change to " + d.Item + ", which has consumers"
		}
	}
	return ""
}

func anyMigration(r *Report, _ Rules) string {
	if len(r.Data) > 0 {
		return fmt.Sprintf("%d migration(s)", len(r.Data))
	}
	return ""
}

// noRuntimeEvidence keeps a report off LOW when nothing confirms there are no unexpected
// consumers (doc 05: "risk level cannot be LOW without runtime evidence for existing consumers").
func noRuntimeEvidence(r *Report, _ Rules) string {
	if len(r.Runtime) == 0 {
		return "no runtime evidence for the affected services"
	}
	for _, c := range r.Consumers {
		if c.Traffic == nil {
			return "no observed traffic for consumer " + c.Consumer + " of " + c.Of
		}
	}
	return ""
}
