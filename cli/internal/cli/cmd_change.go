package cli

import (
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// changeSummary is the one-line view used by `change list` and SessionStart.
type changeSummary struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	State      string  `json:"state"`
	Step       *string `json:"step"`
	LockHolder *string `json:"lock_holder"`
	UpdatedAt  string  `json:"updated_at"`
}

func summarize(c *state.Change) changeSummary {
	s := changeSummary{ID: c.ID, Title: c.Title, State: string(c.State), UpdatedAt: c.UpdatedAt}
	if c.Step != nil {
		s.Step = &c.Step.Name
	}
	if c.Lock != nil {
		s.LockHolder = &c.Lock.Session
	}
	return s
}

// plain converts a change to plain maps with the change.yaml key names, for JSON output.
func plain(c *state.Change) (map[string]any, error) {
	b, err := yaml.Marshal(c)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, "render change", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, "render change", err)
	}
	return m, nil
}

func cmdChangeNew(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("change new")
	title := fs.String("title", "", "change title (required)")
	prdFile := fs.String("prd-file", "", "copy the PRD from this file")
	prdText := fs.String("prd-text", "", "use this text as the PRD")
	prdStdin := fs.Bool("prd-stdin", false, "read the PRD from stdin")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	if strings.TrimSpace(*title) == "" {
		return nil, usagef("change new: --title is required")
	}
	prd, source, err := readPRD(r, *prdFile, *prdText, *prdStdin)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	set := r.cfg.Settings()
	createdBy := set.Identity
	if createdBy == "" {
		createdBy = "unknown"
	}
	c, err := s.Create(state.NewChange{
		Title: *title, CreatedBy: createdBy, PRD: prd, PRDSource: source,
		Budget: state.Budget{MaxCostUSD: set.BudgetMaxCostUSD, MaxHours: set.BudgetMaxHours, MaxToolCalls: set.BudgetMaxToolCalls},
	})
	if err != nil {
		return nil, err
	}
	return summarize(c), nil
}

func cmdChangeList(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("change list")
	all := fs.Bool("all", false, "include done and cancelled changes")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	changes, err := s.List()
	if err != nil {
		return nil, err
	}
	out := []changeSummary{}
	for _, c := range changes {
		if *all || !state.IsTerminal(c.State) {
			out = append(out, summarize(c))
		}
	}
	return out, nil
}

func cmdChangeShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("change show"), args, 1)
	if err != nil {
		return nil, err
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return nil, err
	}
	return plain(c)
}

// stateView is `athel state show`: everything needed to decide the next action.
type stateView struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	State          string            `json:"state"`
	BlockedFrom    *string           `json:"blocked_from"`
	Step           *state.Step       `json:"step"`
	LastCheckpoint *state.Checkpoint `json:"last_checkpoint"`
	Budget         budgetView        `json:"budget"`
	Lock           state.LockStatus  `json:"lock"`
	AllowedNext    []string          `json:"allowed_next"`
	Integrity      string            `json:"integrity"` // ok | edited (frozen until `athel integrity accept`)
}

type budgetView struct {
	Status        string  `json:"status"`
	UsedCostUSD   float64 `json:"used_cost_usd"`
	MaxCostUSD    float64 `json:"max_cost_usd"`
	UsedMinutes   float64 `json:"used_minutes"`
	MaxHours      float64 `json:"max_hours"`
	UsedToolCalls int     `json:"used_tool_calls"`
	MaxToolCalls  int     `json:"max_tool_calls"`
}

func viewOf(c *state.Change, now time.Time) stateView {
	v := stateView{ID: c.ID, Title: c.Title, State: string(c.State), Step: c.Step,
		LastCheckpoint: state.LastCheckpoint(c), Lock: state.Status(c, now), AllowedNext: []string{}, Integrity: "ok"}
	if c.Tampered() {
		v.Integrity = "edited"
	}
	if c.BlockedFrom != nil {
		bf := string(*c.BlockedFrom)
		v.BlockedFrom = &bf
	}
	b := c.Budget
	v.Budget = budgetView{Status: b.Status, UsedCostUSD: b.UsedCostUSD, MaxCostUSD: b.MaxCostUSD, UsedMinutes: b.UsedMinutes,
		MaxHours: b.MaxHours, UsedToolCalls: b.UsedToolCalls, MaxToolCalls: b.MaxToolCalls}
	for _, s := range state.NextStates(c) {
		v.AllowedNext = append(v.AllowedNext, string(s))
	}
	return v
}

// resolveChange picks the change a command acts on: the given ID, else the only non-terminal
// change. With several active changes the caller must name one; the CLI never guesses.
func resolveChange(s *state.Store, pos []string) (*state.Change, error) {
	if len(pos) == 1 {
		return s.Get(pos[0])
	}
	changes, err := s.List()
	if err != nil {
		return nil, err
	}
	var active []*state.Change
	for _, c := range changes {
		if !state.IsTerminal(c.State) {
			active = append(active, c)
		}
	}
	if len(active) != 1 {
		ids := make([]string, len(active))
		for i, c := range active {
			ids[i] = c.ID
		}
		return nil, aerr.New(aerr.StateInvalid, "name the change: "+strconv.Itoa(len(active))+" active changes ["+strings.Join(ids, ", ")+"]")
	}
	return active[0], nil
}

func cmdStateShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("state show"), args, -1)
	if err != nil {
		return nil, err
	}
	if len(pos) > 1 {
		return nil, usagef("state show: expected at most 1 argument")
	}
	s, err := r.storeFor()
	if err != nil {
		return nil, err
	}
	c, err := resolveChange(s, pos)
	if err != nil {
		return nil, err
	}
	return viewOf(c, r.env.Clock.Now()), nil
}
