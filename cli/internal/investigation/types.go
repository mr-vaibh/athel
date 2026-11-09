// Package investigation checks and saves the scout's answers (doc 03 §Scout agent and
// investigation protocol; FR-DISC-001 to 003, 006, 009 to 014). The model writes the claims;
// the CLI decides how sure each claim may be, from its evidence alone (M2 plan §4.3).
package investigation

import "github.com/mr-vaibh/athel/cli/internal/schema"

// SchemaID is the schema of the claim block.
const SchemaID = schema.Investigation

// Investigation is one investigation@1 document. Field order is the YAML output order.
type Investigation struct {
	Schema       string       `yaml:"schema" json:"schema"`
	Change       string       `yaml:"change,omitempty" json:"change,omitempty"`
	Ask          string       `yaml:"ask,omitempty" json:"ask,omitempty"`
	Question     string       `yaml:"question" json:"question"`
	Type         string       `yaml:"type" json:"type"`
	Scope        Scope        `yaml:"scope" json:"scope"`
	Complete     bool         `yaml:"complete" json:"complete"`
	Budget       Budget       `yaml:"budget" json:"budget"`
	Claims       []Claim      `yaml:"claims" json:"claims"`
	Drift        []Drift      `yaml:"drift" json:"drift"`
	Gaps         []Gap        `yaml:"gaps" json:"gaps"`
	ActiveWork   []ActiveWork `yaml:"active_work" json:"active_work"`
	Health       []Health     `yaml:"health" json:"health"`
	Suspicious   []Suspicious `yaml:"suspicious" json:"suspicious"`
	FactsUsed    []string     `yaml:"facts_used" json:"facts_used"`
	FactsCreated []string     `yaml:"facts_created" json:"facts_created"`
}

// Scope is what the question is about.
type Scope struct {
	Services []string `yaml:"services" json:"services"`
	Repos    []string `yaml:"repos" json:"repos"`
}

// Budget is the question budget and what was used (FR-DISC-014).
type Budget struct {
	MaxToolCalls  int     `yaml:"max_tool_calls" json:"max_tool_calls"`
	MaxMinutes    int     `yaml:"max_minutes" json:"max_minutes"`
	UsedToolCalls int     `yaml:"used_tool_calls" json:"used_tool_calls"`
	UsedMinutes   float64 `yaml:"used_minutes" json:"used_minutes"`
}

// Entity is a claim's subject or object.
type Entity map[string]string

// Source is one piece of evidence.
type Source struct {
	Kind      string  `yaml:"kind" json:"kind"`
	Ref       string  `yaml:"ref" json:"ref"`
	Commit    string  `yaml:"commit,omitempty" json:"commit,omitempty"`
	Env       string  `yaml:"env,omitempty" json:"env,omitempty"`
	Window    string  `yaml:"window,omitempty" json:"window,omitempty"`
	Sampling  float64 `yaml:"sampling,omitempty" json:"sampling,omitempty"`
	ToolUseID string  `yaml:"tool_use_id,omitempty" json:"tool_use_id,omitempty"`
}

// Claim is one statement with its evidence and confidence.
type Claim struct {
	ID         string   `yaml:"id" json:"id"`
	Text       string   `yaml:"text" json:"text"`
	Subject    Entity   `yaml:"subject" json:"subject"`
	Predicate  string   `yaml:"predicate" json:"predicate"`
	Object     Entity   `yaml:"object" json:"object"`
	Env        string   `yaml:"env" json:"env"`
	Confidence string   `yaml:"confidence" json:"confidence"`
	Evidence   []Source `yaml:"evidence" json:"evidence"`
	Facts      []string `yaml:"facts" json:"facts"`
}

// Drift is a disagreement between sources (FR-DISC-006).
type Drift struct {
	Text     string   `yaml:"text" json:"text"`
	Evidence []Source `yaml:"evidence" json:"evidence"`
}

// Gap is a source that could not be used and what that costs the answer (FR-DISC-009).
type Gap struct {
	Source string  `yaml:"source" json:"source"`
	Code   string  `yaml:"code" json:"code"`
	Reason string  `yaml:"reason" json:"reason"`
	Login  *string `yaml:"login" json:"login"`
	Effect string  `yaml:"effect" json:"effect"`
}

// ActiveWork is someone else's work on the services in scope (FR-DISC-010).
type ActiveWork struct {
	Kind    string   `yaml:"kind" json:"kind"`
	Ref     string   `yaml:"ref" json:"ref"`
	Author  *string  `yaml:"author" json:"author"`
	Touches []string `yaml:"touches" json:"touches"`
}

// Health is the current health of one service in one environment (FR-DISC-011).
type Health struct {
	Service       string   `yaml:"service" json:"service"`
	Env           string   `yaml:"env" json:"env"`
	Window        string   `yaml:"window" json:"window"`
	ErrorRate     *float64 `yaml:"error_rate" json:"error_rate"`
	LatencyMS     *Latency `yaml:"latency_ms" json:"latency_ms"`
	Alerts        []Alert  `yaml:"alerts" json:"alerts"`
	Incidents     []string `yaml:"incidents" json:"incidents"`
	RecentDeploys []string `yaml:"recent_deploys" json:"recent_deploys"`
	Evidence      []Source `yaml:"evidence" json:"evidence"`
}

// Latency percentiles in milliseconds.
type Latency struct {
	P50 *float64 `yaml:"p50,omitempty" json:"p50,omitempty"`
	P95 *float64 `yaml:"p95,omitempty" json:"p95,omitempty"`
	P99 *float64 `yaml:"p99,omitempty" json:"p99,omitempty"`
}

// Alert is one alert on a service.
type Alert struct {
	Name     string `yaml:"name" json:"name"`
	Severity string `yaml:"severity" json:"severity"`
	Firing   bool   `yaml:"firing" json:"firing"`
}

// Suspicious is tool output that read like instructions to an AI and was treated as data.
type Suspicious struct {
	Ref       string `yaml:"ref" json:"ref"`
	Excerpt   string `yaml:"excerpt" json:"excerpt"`
	ToolUseID string `yaml:"tool_use_id,omitempty" json:"tool_use_id,omitempty"`
}
