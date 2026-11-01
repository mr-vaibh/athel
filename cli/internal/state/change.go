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
	Schema       string         `yaml:"schema"`
	ID           string         `yaml:"id"`
	Title        string         `yaml:"title"`
	State        State          `yaml:"state"`
	BlockedFrom  *State         `yaml:"blocked_from"`
	CreatedBy    string         `yaml:"created_by"`
	CreatedAt    string         `yaml:"created_at"`
	UpdatedAt    string         `yaml:"updated_at"`
	PRD          PRD            `yaml:"prd"`
	Step         *Step          `yaml:"step"`
	StepAttempts map[string]int `yaml:"step_attempts"`
	StopBlocks   StopBlocks     `yaml:"stop_blocks"`
	Checkpoints  []Checkpoint   `yaml:"checkpoints"`
	Budget       Budget         `yaml:"budget"`
	Lock         *Lock          `yaml:"lock"`
	Approvals    Approvals      `yaml:"approvals"`
	Waivers      []any          `yaml:"waivers"`
	Repos        []any          `yaml:"repos"`
	ChangeSet    any            `yaml:"change_set"`
	Parent       any            `yaml:"parent"`
	Flags        []any          `yaml:"flags"`
	History      []Transition   `yaml:"history"`
}

// PRD lists the PRD versions of a change; each UAT round may add one (doc 07).
type PRD struct {
	Current  string       `yaml:"current"`
	Versions []PRDVersion `yaml:"versions"`
}

// PRDVersion is one stored PRD file and its hash.
type PRDVersion struct {
	Version string `yaml:"version"`
	Path    string `yaml:"path"`
	SHA256  string `yaml:"sha256"`
	Source  string `yaml:"source"`
	At      string `yaml:"at"`
}

// Step is the workflow step currently or last run.
type Step struct {
	Name      string  `yaml:"name"`
	Task      *string `yaml:"task"`
	StartedAt string  `yaml:"started_at"`
	EndedAt   *string `yaml:"ended_at"`
	Attempt   int     `yaml:"attempt"`
	Status    string  `yaml:"status"`
	Reason    *string `yaml:"reason"`
}

// StopBlocks counts consecutive Stop-hook blocks; doc 02 moves the step to blocked after 3.
type StopBlocks struct {
	Count int `yaml:"count"`
}

// Checkpoint marks progress inside a step; resume restarts at the last one.
type Checkpoint struct {
	Name    string  `yaml:"name"`
	Step    *string `yaml:"step"`
	At      string  `yaml:"at"`
	Note    *string `yaml:"note"`
	Session string  `yaml:"session"`
	Trigger string  `yaml:"trigger"`
}

// Budget limits and usage (doc 02 §Runtime model). M0 records and reports; nothing enforces.
type Budget struct {
	MaxCostUSD    float64       `yaml:"max_cost_usd"`
	MaxHours      float64       `yaml:"max_hours"`
	MaxToolCalls  int           `yaml:"max_tool_calls"`
	UsedCostUSD   float64       `yaml:"used_cost_usd"`
	UsedMinutes   float64       `yaml:"used_minutes"`
	UsedToolCalls int           `yaml:"used_tool_calls"`
	Status        string        `yaml:"status"`
	Events        []BudgetEvent `yaml:"events"`
}

// BudgetEvent records a budget dimension crossing into a new status.
type BudgetEvent struct {
	At        string `yaml:"at"`
	Status    string `yaml:"status"`
	Dimension string `yaml:"dimension"`
}

// Lock is the logical one-writer-per-change session lock (doc 02 §Locking).
type Lock struct {
	Session     string `yaml:"session"`
	Since       string `yaml:"since"`
	HeartbeatAt string `yaml:"heartbeat_at"`
	TTLMin      int    `yaml:"ttl_min"`
}

// Approvals is reserved for the plan approval bound to the plan hash (M3).
type Approvals struct {
	Plan any `yaml:"plan"`
}

// Transition is one append-only history entry.
type Transition struct {
	Seq      int        `yaml:"seq"`
	From     State      `yaml:"from"`
	To       State      `yaml:"to"`
	By       string     `yaml:"by"`
	At       string     `yaml:"at"`
	Reason   string     `yaml:"reason"`
	Evidence []Evidence `yaml:"evidence"`
	Origin   Origin     `yaml:"origin"`
}

// Evidence is a file that justified a transition and its hash at that moment.
type Evidence struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

// Origin records where a transition command came from, so M1 can enforce that human-only
// transitions originate from a human prompt rather than a subagent.
type Origin struct {
	Session  *string `yaml:"session"`
	PromptID *string `yaml:"prompt_id"`
}
