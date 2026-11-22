// Package plan reads and checks plan.yaml (athel/plan@1, doc 05 §The change plan) and matches
// writes against its tasks (doc 04 rule 7). Plan authoring is milestone M3; M1 needs the format,
// the hash approval binds to, and the matcher.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// SchemaID is the schema plan.yaml is written in; SchemaV1 plans (M1) stay readable, with the
// same hash, so an approval given to one stays valid (ADR 020).
const (
	SchemaID = "athel/plan@2"
	SchemaV1 = "athel/plan@1"
)

// FileName is the plan's file inside the change folder.
const FileName = "plan.yaml"

// Task is one node of the task graph.
type Task struct {
	ID         string   `yaml:"id" json:"id"`
	Title      string   `yaml:"title" json:"title"`
	Repo       string   `yaml:"repo" json:"repo"`
	Paths      []string `yaml:"paths" json:"paths"`
	Resources  []string `yaml:"resources,omitempty" json:"resources,omitempty"`
	Verbs      []string `yaml:"verbs" json:"verbs"`
	Covers     []string `yaml:"covers" json:"covers"`
	DependsOn  []string `yaml:"depends_on" json:"depends_on"`
	Size       string   `yaml:"size" json:"size"`
	Gates      []string `yaml:"gates" json:"gates"`
	RiskNotes  string   `yaml:"risk_notes,omitempty" json:"risk_notes,omitempty"`
	Rollback   string   `yaml:"rollback,omitempty" json:"rollback,omitempty"`
	EnvActions []string `yaml:"env_actions,omitempty" json:"env_actions,omitempty"`
	Kind       string   `yaml:"kind,omitempty" json:"kind,omitempty"`
	SizeReason string   `yaml:"size_reason,omitempty" json:"size_reason,omitempty"`
	Deferred   bool     `yaml:"deferred,omitempty" json:"deferred,omitempty"`
}

// Plan is plan.yaml.
type Plan struct {
	Schema string  `yaml:"schema" json:"schema"`
	Change string  `yaml:"change" json:"change"`
	Hash   *string `yaml:"hash" json:"hash"`
	Tasks  []Task  `yaml:"tasks" json:"tasks"`
}

// Decode validates data against the plan schema it names (plan@2, or plan@1 from M1) and
// checks the task graph: unique task IDs, dependencies that exist, and no cycles (FR-PLAN-001).
func Decode(reg *schema.Registry, data []byte) (*Plan, error) {
	var head struct {
		Schema string `yaml:"schema"`
	}
	_ = yaml.Unmarshal(data, &head) // a malformed document fails the schema check below
	id := SchemaID
	if head.Schema == SchemaV1 {
		id = SchemaV1
	}
	if err := reg.ValidateYAML(id, data); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode plan", err)
	}
	if err := p.checkGraph(); err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *Plan) checkGraph() error {
	deps := map[string][]string{}
	for _, t := range p.Tasks {
		if _, dup := deps[t.ID]; dup {
			return aerr.New(aerr.StateInvalid, "plan: task "+t.ID+" is listed twice")
		}
		deps[t.ID] = t.DependsOn
	}
	for id, ds := range deps {
		for _, d := range ds {
			if _, ok := deps[d]; !ok {
				return aerr.New(aerr.StateInvalid, fmt.Sprintf("plan: task %s depends on unknown task %s", id, d))
			}
		}
	}
	state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var visit func(string) bool
	visit = func(id string) bool {
		switch state[id] {
		case 1:
			return false
		case 2:
			return true
		}
		state[id] = 1
		for _, d := range deps[id] {
			if !visit(d) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if !visit(id) {
			return aerr.New(aerr.StateInvalid, "plan: the task graph has a cycle through "+id)
		}
	}
	return nil
}

// ComputeHash is the hash approval binds to: the canonical JSON of the plan without its hash.
func ComputeHash(p *Plan) string {
	c := *p
	c.Hash = nil
	b, _ := json.Marshal(c) // only strings and slices of strings: encoding cannot fail
	return digest.Bytes(b)
}

// Encode renders the plan with its hash filled in.
func Encode(p *Plan) ([]byte, error) {
	h := ComputeHash(p)
	p.Hash = &h
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(p); err != nil {
		return nil, aerr.Wrap(aerr.StateInvalid, "encode plan", err)
	}
	return buf.Bytes(), enc.Close()
}

// Write is one write to match against the plan.
type Write struct {
	Repo string // repository (worktree) name
	Path string // path relative to the repository root ("" for repo-level actions like push)
	Verb string // edit | create | delete | test | commit | push | pr | comment
}

// WritesIn reports whether any task of the plan writes in repo.
func (p *Plan) WritesIn(repo string) bool {
	return slices.ContainsFunc(p.Tasks, func(t Task) bool { return t.Repo == repo })
}

// Match returns the first task that allows w, or nil.
func (p *Plan) Match(w Write) *Task {
	for i := range p.Tasks {
		t := &p.Tasks[i]
		if t.Repo != w.Repo || !slices.Contains(t.Verbs, w.Verb) {
			continue
		}
		if w.Path == "" {
			return t
		}
		for _, g := range t.Paths {
			if MatchGlob(g, w.Path) {
				return t
			}
		}
	}
	return nil
}

// MatchGlob matches a slash-separated path against a pattern where "*" matches within one
// segment and "**" matches any number of segments.
func MatchGlob(pattern, path string) bool {
	return matchSegs(strings.Split(strings.Trim(pattern, "/"), "/"), strings.Split(strings.Trim(path, "/"), "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 || !matchSeg(pat[0], segs[0]) {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

func matchSeg(pattern, seg string) bool {
	if pattern == seg {
		return true
	}
	star := strings.Index(pattern, "*")
	if star < 0 {
		return false
	}
	prefix, rest := pattern[:star], pattern[star+1:]
	if !strings.HasPrefix(seg, prefix) {
		return false
	}
	seg = seg[len(prefix):]
	for i := 0; i <= len(seg); i++ {
		if matchSeg(rest, seg[i:]) {
			return true
		}
	}
	return false
}
