// Package facts is the verified knowledge cache (doc 03 §Verified knowledge cache): facts with
// sources and a re-check recipe, freshness windows, and a log of every use. Only the CLI writes
// it, and a fact is a hint until it has been verified within its window.
package facts

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Schemas of the cache files.
const (
	SchemaID       = schema.Fact
	AccessSchemaID = schema.FactAccess
)

// Freshness classes (doc 03 §Freshness classes).
const (
	Volatile       = "volatile"
	Runtime        = "runtime"
	Topology       = "topology"
	Structural     = "structural"
	Organisational = "organisational"
)

// Fact statuses.
const (
	Active     = "active"
	Superseded = "superseded"
	Disputed   = "disputed"
)

// Entity is the subject or object of a fact: a kind, a name and optional string attributes.
type Entity map[string]string

// Source is one piece of evidence: a tool reference, never free text (anti-poisoning).
type Source struct {
	Kind      string  `json:"kind" yaml:"kind"`
	Ref       string  `json:"ref" yaml:"ref"`
	Commit    string  `json:"commit,omitempty" yaml:"commit,omitempty"`
	Env       string  `json:"env,omitempty" yaml:"env,omitempty"`
	Window    string  `json:"window,omitempty" yaml:"window,omitempty"`
	Sampling  float64 `json:"sampling,omitempty" yaml:"sampling,omitempty"`
	ToolUseID string  `json:"tool_use_id,omitempty" yaml:"tool_use_id,omitempty"`
}

// Step is one recipe step: tool, action and its parameters.
type Step map[string]any

// Fact is one fact@1 record. Field order is the JSON output order.
type Fact struct {
	Schema         string   `json:"schema"`
	ID             string   `json:"id"`
	Subject        Entity   `json:"subject"`
	Predicate      string   `json:"predicate"`
	Object         Entity   `json:"object"`
	Env            string   `json:"env"`
	Confidence     string   `json:"confidence"`
	Sources        []Source `json:"sources"`
	Verify         []Step   `json:"verify"`
	FreshnessClass string   `json:"freshness_class"`
	VerifiedAt     string   `json:"verified_at"`
	CreatedAt      string   `json:"created_at"`
	Status         string   `json:"status"`
	Supersedes     *string  `json:"supersedes"`
	DerivedFrom    []string `json:"derived_from"`
}

// Draft is a fact as a caller supplies it. The CLI owns the id, timestamps, status and
// supersedes, so a draft that sets them is rejected.
type Draft struct {
	Subject        Entity   `yaml:"subject"`
	Predicate      string   `yaml:"predicate"`
	Object         Entity   `yaml:"object"`
	Env            string   `yaml:"env"`
	Confidence     string   `yaml:"confidence"`
	Sources        []Source `yaml:"sources"`
	Verify         []Step   `yaml:"verify"`
	FreshnessClass string   `yaml:"freshness_class"`
	DerivedFrom    []string `yaml:"derived_from"`
}

// DecodeDraft parses one YAML (or JSON) draft, rejecting unknown fields and extra documents.
func DecodeDraft(data []byte) (Draft, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var d Draft
	if err := dec.Decode(&d); err != nil {
		return Draft{}, aerr.Wrap(aerr.StateInvalid, "fact draft (the CLI assigns id, timestamps, status and supersedes)", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return Draft{}, aerr.New(aerr.StateInvalid, "fact draft: more than one document")
	}
	return d, nil
}

// fact turns a draft into a record without the CLI-owned fields. Slices are never nil, so the
// record always encodes as the schema expects.
func (d Draft) fact() Fact {
	f := Fact{
		Schema: SchemaID, Subject: d.Subject, Predicate: d.Predicate, Object: d.Object, Env: d.Env,
		Confidence: d.Confidence, Sources: d.Sources, Verify: d.Verify, FreshnessClass: d.FreshnessClass,
		DerivedFrom: d.DerivedFrom,
	}
	if f.DerivedFrom == nil {
		f.DerivedFrom = []string{}
	}
	return f
}

// key is the de-duplication key (doc 03 verification rule 5): subject, predicate, object, env.
// Entity attributes are sorted and quoted, so equal facts always produce the same key.
func (f Fact) key() string {
	return strings.Join([]string{f.Subject.canonical(), strconv.Quote(f.Predicate), f.Object.canonical(), strconv.Quote(f.Env)}, "|")
}

func (e Entity) canonical() string {
	parts := make([]string, 0, len(e))
	for _, k := range slices.Sorted(maps.Keys(e)) {
		parts = append(parts, strconv.Quote(k)+"="+strconv.Quote(e[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
