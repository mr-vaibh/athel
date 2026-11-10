// Package decisions keeps decisions.yaml (athel/decision@1): decision cards raised by an ask
// rule, and the choices Athel made alone (doc 05 §Decision cards; FR-DEC-001…004). IDs and
// times come from the CLI; the lint covers what the schema cannot.
package decisions

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Option is one answer a card offers.
type Option struct {
	Key         string `yaml:"key" json:"key"`
	Label       string `yaml:"label" json:"label"`
	Consequence string `yaml:"consequence" json:"consequence"`
}

// Recommendation is the option Athel recommends and why.
type Recommendation struct {
	Option string `yaml:"option" json:"option"`
	Reason string `yaml:"reason" json:"reason"`
}

// Answer is how a card was answered: by the human's /athel:decide prompt or an applied default.
type Answer struct {
	Option  string `yaml:"option,omitempty" json:"option,omitempty"`
	Text    string `yaml:"text,omitempty" json:"text,omitempty"`
	By      string `yaml:"by" json:"by"`
	At      string `yaml:"at" json:"at"`
	Grant   string `yaml:"grant,omitempty" json:"grant,omitempty"`
	Checked *bool  `yaml:"checked,omitempty" json:"checked,omitempty"`
}

// Card is a question for the human.
type Card struct {
	ID                 string         `yaml:"id" json:"id"`
	Kind               string         `yaml:"kind" json:"kind"`
	Rule               string         `yaml:"rule" json:"rule"`
	Step               string         `yaml:"step" json:"step"`
	Question           string         `yaml:"question" json:"question"`
	Context            string         `yaml:"context" json:"context"`
	Options            []Option       `yaml:"options" json:"options"`
	Recommendation     Recommendation `yaml:"recommendation" json:"recommendation"`
	Default            string         `yaml:"default_if_unanswered" json:"default_if_unanswered"`
	DefaultSafe        bool           `yaml:"default_safe" json:"default_safe"`
	Blocks             []string       `yaml:"blocks" json:"blocks"`
	NewBlocker         bool           `yaml:"new_blocker,omitempty" json:"new_blocker,omitempty"`
	PresentedAt        *string        `yaml:"presented_at" json:"presented_at"`
	Answer             *Answer        `yaml:"answer" json:"answer"`
	ShouldNotHaveAsked *bool          `yaml:"should_not_have_asked,omitempty" json:"should_not_have_asked,omitempty"`
}

// Draft is a card as its author writes it. The fields the CLI owns (id, kind, presented_at,
// answer, feedback) are not in it, so a draft that sets them is refused, not quietly cleaned.
type Draft struct {
	Rule           string         `yaml:"rule"`
	Step           string         `yaml:"step"`
	Question       string         `yaml:"question"`
	Context        string         `yaml:"context"`
	Options        []Option       `yaml:"options"`
	Recommendation Recommendation `yaml:"recommendation"`
	Default        string         `yaml:"default_if_unanswered"`
	DefaultSafe    bool           `yaml:"default_safe"`
	Blocks         []string       `yaml:"blocks"`
	NewBlocker     bool           `yaml:"new_blocker"`
}

// DecidedDraft is a choice as Athel's agent writes it; the CLI adds id, kind, decided_by and at.
type DecidedDraft struct {
	Step     string `yaml:"step"`
	Question string `yaml:"question"`
	Choice   string `yaml:"choice"`
	Reason   string `yaml:"reason"`
}

// Decided is a choice Athel made alone, recorded so reviewers can challenge it.
type Decided struct {
	ID        string `yaml:"id" json:"id"`
	Kind      string `yaml:"kind" json:"kind"`
	Step      string `yaml:"step" json:"step"`
	Question  string `yaml:"question" json:"question"`
	Choice    string `yaml:"choice" json:"choice"`
	Reason    string `yaml:"reason" json:"reason"`
	DecidedBy string `yaml:"decided_by" json:"decided_by"`
	At        string `yaml:"at" json:"at"`
}

// Entry is one item of decisions.yaml: exactly one of Card and Decided is set.
type Entry struct {
	Card    *Card
	Decided *Decided
}

// ID is the entry's D-n.
func (e Entry) ID() string {
	if e.Card != nil {
		return e.Card.ID
	}
	return e.Decided.ID
}

// MarshalYAML writes the entry as its card or decision.
func (e Entry) MarshalYAML() (any, error) {
	if e.Card != nil {
		return e.Card, nil
	}
	return e.Decided, nil
}

// MarshalJSON writes the entry as its card or decision.
func (e Entry) MarshalJSON() ([]byte, error) {
	if e.Card != nil {
		return jsonOf(e.Card)
	}
	return jsonOf(e.Decided)
}

// UnmarshalYAML reads a card or a decision by its kind.
func (e *Entry) UnmarshalYAML(n *yaml.Node) error {
	var probe struct {
		Kind string `yaml:"kind"`
	}
	if err := n.Decode(&probe); err != nil {
		return err
	}
	if probe.Kind == "card" {
		e.Card = &Card{}
		return n.Decode(e.Card)
	}
	e.Decided = &Decided{}
	return n.Decode(e.Decided)
}

// File is decisions.yaml.
type File struct {
	Schema    string  `yaml:"schema" json:"schema"`
	Change    string  `yaml:"change" json:"change"`
	Decisions []Entry `yaml:"decisions" json:"decisions"`
}

// New is an empty decisions file for a change.
func New(change string) *File {
	return &File{Schema: schema.Decision, Change: change, Decisions: []Entry{}}
}

// Decode reads decisions.yaml, or returns an empty file for a change that has none yet.
func Decode(reg *schema.Registry, change string, data []byte) (*File, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return New(change), nil
	}
	if err := reg.ValidateYAML(schema.Decision, data); err != nil {
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode decisions.yaml", err)
	}
	return &f, nil
}

// Encode writes decisions.yaml and checks the result against the schema, so a bug can never
// store an invalid file.
func (f *File) Encode(reg *schema.Registry) ([]byte, error) {
	b, err := yaml.Marshal(f)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, "encode decisions.yaml", err)
	}
	if err := reg.ValidateYAML(schema.Decision, b); err != nil {
		return nil, err
	}
	return b, nil
}

// nextID is the next free D-n; IDs are never reused.
func (f *File) nextID() string {
	n := 0
	for _, e := range f.Decisions {
		v, _ := strconv.Atoi(strings.TrimPrefix(e.ID(), "D-")) // schema: ^D-[0-9]+$
		n = max(n, v)
	}
	return "D-" + strconv.Itoa(n+1)
}

// Card finds a card by ID.
func (f *File) Card(id string) (*Card, bool) {
	for _, e := range f.Decisions {
		if e.Card != nil && e.Card.ID == id {
			return e.Card, true
		}
	}
	return nil, false
}

// Lint lists what the schema cannot check on a card: option keys a, b, c… in order, a
// recommendation and a default that name an offered option, and no open card asking the same
// question.
func (f *File) Lint(c *Card) []string {
	var problems []string
	keys := make([]string, 0, len(c.Options))
	for i, o := range c.Options {
		if want := string(rune('a' + i)); o.Key != want {
			problems = append(problems, fmt.Sprintf("option %d has key %q; keys are a, b, c, d in order", i+1, o.Key))
		}
		keys = append(keys, o.Key)
	}
	if !slices.Contains(keys, c.Recommendation.Option) {
		problems = append(problems, "the recommendation names option "+c.Recommendation.Option+", which the card does not offer")
	}
	if !slices.Contains(keys, c.Default) {
		problems = append(problems, "default_if_unanswered names option "+c.Default+", which the card does not offer")
	}
	for _, e := range f.Decisions {
		if e.Card != nil && e.Card.Answer == nil && strings.EqualFold(e.Card.Question, c.Question) {
			problems = append(problems, "open card "+e.Card.ID+" already asks this question")
		}
	}
	return problems
}

// Add gives a draft its ID and the fields the CLI owns, lints it and appends it.
func (f *File) Add(d Draft) (*Card, error) {
	c := Card{Kind: "card", Rule: d.Rule, Step: d.Step, Question: d.Question, Context: d.Context,
		Options: d.Options, Recommendation: d.Recommendation, Default: d.Default, DefaultSafe: d.DefaultSafe,
		Blocks: d.Blocks, NewBlocker: d.NewBlocker}
	if c.Blocks == nil {
		c.Blocks = []string{}
	}
	if p := f.Lint(&c); len(p) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "card rejected: "+strings.Join(p, "; ")).
			WithNextAction("fix the card and add it again")
	}
	c.ID = f.nextID()
	f.Decisions = append(f.Decisions, Entry{Card: &c})
	return &c, nil
}

// Record appends a choice Athel made alone (doc 05: "still recorded … so reviewers can challenge
// it").
func (f *File) Record(d DecidedDraft, at string) *Decided {
	rec := &Decided{ID: f.nextID(), Kind: "decision", Step: d.Step, Question: d.Question, Choice: d.Choice,
		Reason: d.Reason, DecidedBy: "athel", At: at}
	f.Decisions = append(f.Decisions, Entry{Decided: rec})
	return rec
}

// Open lists the unanswered cards of a step, or of every step when step is "".
func (f *File) Open(step string) []*Card {
	var out []*Card
	for _, e := range f.Decisions {
		if e.Card != nil && e.Card.Answer == nil && (step == "" || e.Card.Step == step) {
			out = append(out, e.Card)
		}
	}
	return out
}

// Present returns a step's open cards as one batch, shortest question first (doc 05
// §Presentation), and marks them presented. A step interrupts the human once (FR-DEC-003): once
// any card of the step was presented, new cards may join a later batch only as new blockers.
func (f *File) Present(step, at string) ([]*Card, error) {
	open := f.Open(step)
	interrupted := false
	for _, e := range f.Decisions {
		if e.Card != nil && e.Card.Step == step && e.Card.PresentedAt != nil {
			interrupted = true
		}
	}
	var late []string
	for _, c := range open {
		if c.PresentedAt == nil && interrupted && !c.NewBlocker {
			late = append(late, c.ID)
		}
	}
	if len(late) > 0 {
		return nil, aerr.New(aerr.StateInvalid, "step "+step+" already asked the human once; "+strings.Join(late, ", ")+
			" may join only as a new blocker (FR-DEC-003)").
			WithNextAction("decide it alone with `athel decide record` if the ask rules allow, or mark it new_blocker: true if it blocks work")
	}
	slices.SortStableFunc(open, func(a, b *Card) int { return cmp.Compare(len(a.Question), len(b.Question)) })
	for _, c := range open {
		if c.PresentedAt == nil {
			c.PresentedAt = &at
		}
	}
	return open, nil
}

func jsonOf(v any) ([]byte, error) { return json.Marshal(v) }
