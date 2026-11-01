// Package state owns the change model: the change.yaml document, its lifecycle, steps,
// checkpoints, budgets and session lock. Only this package writes change.yaml, always under an
// OS file lock and always schema-validated before the atomic write.
package state

// State is a lifecycle state (doc 02 §State model).
type State string

// Lifecycle states.
const (
	Draft             State = "draft"
	Investigating     State = "investigating"
	Specified         State = "specified"
	AwaitingDecisions State = "awaiting_decisions"
	Planned           State = "planned"
	PlanApproved      State = "plan_approved"
	Building          State = "building"
	ReadyForPR        State = "ready_for_pr"
	InReview          State = "in_review"
	Verifying         State = "verifying"
	UAT               State = "uat"
	Done              State = "done"
	Blocked           State = "blocked"
	Cancelled         State = "cancelled"
)

// SchemaID is the schema of change.yaml.
const SchemaID = "athel/change@1"

// Change is the content of change.yaml. Field order is the YAML output order. Timestamps are
// kept as strings in clock.Layout so the file round-trips byte for byte.
type Change struct {
	Schema       string         `yaml:"schema" json:"schema"`
	ID           string         `yaml:"id" json:"id"`
	Title        string         `yaml:"title" json:"title"`
	State        State          `yaml:"state" json:"state"`
	BlockedFrom  *State         `yaml:"blocked_from" json:"blocked_from"`
	CreatedBy    string         `yaml:"created_by" json:"created_by"`
	CreatedAt    string         `yaml:"created_at" json:"created_at"`
	UpdatedAt    string         `yaml:"updated_at" json:"updated_at"`
	PRD          PRD            `yaml:"prd" json:"prd"`
	Step         *Step          `yaml:"step" json:"step"`
	StepAttempts map[string]int `yaml:"step_attempts" json:"step_attempts"`
	StopBlocks   StopBlocks     `yaml:"stop_blocks" json:"stop_blocks"`
	Checkpoints  []Checkpoint   `yaml:"checkpoints" json:"checkpoints"`
	Budget       Budget         `yaml:"budget" json:"budget"`
	Lock         *Lock          `yaml:"lock" json:"lock"`
	Approvals    Approvals      `yaml:"approvals" json:"approvals"`
	Waivers      []any          `yaml:"waivers" json:"waivers"`
	Repos        []any          `yaml:"repos" json:"repos"`
	ChangeSet    any            `yaml:"change_set" json:"change_set"`
	Parent       any            `yaml:"parent" json:"parent"`
	Flags        []any          `yaml:"flags" json:"flags"`
	History      []Transition   `yaml:"history" json:"history"`
}

// PRD lists the PRD versions of a change; each UAT round may add one (doc 07).
type PRD struct {
	Current  string       `yaml:"current" json:"current"`
	Versions []PRDVersion `yaml:"versions" json:"versions"`
}

// PRDVersion is one stored PRD file and its hash.
type PRDVersion struct {
	Version string `yaml:"version" json:"version"`
	Path    string `yaml:"path" json:"path"`
	SHA256  string `yaml:"sha256"`
	Source  string `yaml:"source" json:"source"`
	At      string `yaml:"at" json:"at"`
}

// Step is the workflow step currently or last run.
type Step struct {
	Name      string  `yaml:"name" json:"name"`
	Task      *string `yaml:"task" json:"task"`
	StartedAt string  `yaml:"started_at" json:"started_at"`
	EndedAt   *string `yaml:"ended_at" json:"ended_at"`
	Attempt   int     `yaml:"attempt" json:"attempt"`
	Status    string  `yaml:"status" json:"status"`
	Reason    *string `yaml:"reason" json:"reason"`
}

// StopBlocks counts consecutive Stop-hook blocks; doc 02 moves the step to blocked after 3.
type StopBlocks struct {
	Count int `yaml:"count" json:"count"`
}

// Checkpoint marks progress inside a step; resume restarts at the last one.
type Checkpoint struct {
	Name    string  `yaml:"name" json:"name"`
	Step    *string `yaml:"step" json:"step"`
	At      string  `yaml:"at" json:"at"`
	Note    *string `yaml:"note" json:"note"`
	Session string  `yaml:"session" json:"session"`
	Trigger string  `yaml:"trigger" json:"trigger"`
}

// Budget limits and usage (doc 02 §Runtime model). M0 records and reports; nothing enforces.
type Budget struct {
	MaxCostUSD    float64       `yaml:"max_cost_usd" json:"max_cost_usd"`
	MaxHours      float64       `yaml:"max_hours" json:"max_hours"`
	MaxToolCalls  int           `yaml:"max_tool_calls" json:"max_tool_calls"`
	UsedCostUSD   float64       `yaml:"used_cost_usd" json:"used_cost_usd"`
	UsedMinutes   float64       `yaml:"used_minutes" json:"used_minutes"`
	UsedToolCalls int           `yaml:"used_tool_calls" json:"used_tool_calls"`
	Status        string        `yaml:"status" json:"status"`
	Events        []BudgetEvent `yaml:"events" json:"events"`
}

// BudgetEvent records a budget dimension crossing into a new status.
type BudgetEvent struct {
	At        string `yaml:"at" json:"at"`
	Status    string `yaml:"status" json:"status"`
	Dimension string `yaml:"dimension" json:"dimension"`
}

// Lock is the logical one-writer-per-change session lock (doc 02 §Locking).
type Lock struct {
	Session     string `yaml:"session" json:"session"`
	Since       string `yaml:"since" json:"since"`
	HeartbeatAt string `yaml:"heartbeat_at" json:"heartbeat_at"`
	TTLMin      int    `yaml:"ttl_min" json:"ttl_min"`
}

// Approvals is reserved for the plan approval bound to the plan hash (M3).
type Approvals struct {
	Plan any `yaml:"plan" json:"plan"`
}

// Transition is one append-only history entry.
type Transition struct {
	Seq      int        `yaml:"seq" json:"seq"`
	From     State      `yaml:"from" json:"from"`
	To       State      `yaml:"to" json:"to"`
	By       string     `yaml:"by" json:"by"`
	At       string     `yaml:"at" json:"at"`
	Reason   string     `yaml:"reason" json:"reason"`
	Evidence []Evidence `yaml:"evidence" json:"evidence"`
	Origin   Origin     `yaml:"origin" json:"origin"`
}

// Evidence is a file that justified a transition and its hash at that moment.
type Evidence struct {
	Path   string `yaml:"path" json:"path"`
	SHA256 string `yaml:"sha256"`
}

// Origin records where a transition command came from, so M1 can enforce that human-only
// transitions originate from a human prompt rather than a subagent.
type Origin struct {
	Session  *string `yaml:"session" json:"session"`
	PromptID *string `yaml:"prompt_id" json:"prompt_id"`
}
