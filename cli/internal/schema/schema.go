// Package schema validates Athel documents against the versioned JSON Schemas embedded in the
// binary. A schema ID has the form athel/<name>@<version>, e.g. athel/change@1.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	athel "github.com/mr-vaibh/athel"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// The schema IDs M0 knows about.
const (
	Change    = "athel/change@1"
	Config    = "athel/config@1"
	HookEvent = "athel/hook-event@1" // M0 log format, kept to read old files
	Audit     = "athel/audit@1"
)

var idRe = regexp.MustCompile(`^athel/([a-z][a-z0-9-]*@[1-9][0-9]*)$`)

// resourceBase is a deliberately unresolvable host: schemas are only ever loaded from the
// embedded files, never fetched.
const resourceBase = "https://schemas.athel.invalid/"

// Registry compiles schemas on first use and caches them. It is safe for concurrent use.
type Registry struct {
	files    fs.FS
	mu       sync.Mutex
	compiled map[string]*jsonschema.Schema
}

// NewRegistry returns a registry over the schemas embedded in the binary.
func NewRegistry() *Registry { return newRegistry(athel.Schemas) }

func newRegistry(files fs.FS) *Registry {
	return &Registry{files: files, compiled: map[string]*jsonschema.Schema{}}
}

// IDs lists the schema IDs the registry can validate, sorted.
func (r *Registry) IDs() ([]string, error) {
	names, err := fs.Glob(r.files, "schemas/*.json")
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "list embedded schemas", err)
	}
	ids := make([]string, 0, len(names))
	for _, n := range names {
		ids = append(ids, "athel/"+strings.TrimSuffix(strings.TrimPrefix(n, "schemas/"), ".json"))
	}
	slices.Sort(ids)
	return ids, nil
}

func (r *Registry) schema(id string) (*jsonschema.Schema, error) {
	m := idRe.FindStringSubmatch(id)
	if m == nil {
		return nil, aerr.New(aerr.StateSchema, fmt.Sprintf("%q is not a schema ID (want athel/<name>@<version>)", id))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.compiled[id]; ok {
		return s, nil
	}
	raw, err := fs.ReadFile(r.files, "schemas/"+m[1]+".json")
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "unknown schema "+id, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "schema "+id+" is not JSON", err)
	}
	c := jsonschema.NewCompiler()
	url := resourceBase + m[1] + ".json"
	if err := c.AddResource(url, doc); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "load schema "+id, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "compile schema "+id, err)
	}
	r.compiled[id] = s
	return s, nil
}

// Validate checks doc (any JSON-compatible value) against schema id.
func (r *Registry) Validate(id string, doc any) error {
	s, err := r.schema(id)
	if err != nil {
		return err
	}
	inst, err := toJSONValue(doc)
	if err != nil {
		return aerr.Wrap(aerr.StateSchema, "document for "+id+" is not representable as JSON", err)
	}
	if err := s.Validate(inst); err != nil {
		return problemsError(id, err)
	}
	return nil
}

// ValidateYAML parses data as YAML and validates it against schema id.
func (r *Registry) ValidateYAML(id string, data []byte) error {
	doc, err := DecodeYAML(data)
	if err != nil {
		return err
	}
	return r.Validate(id, doc)
}

// DecodeYAML parses YAML into plain values, rejecting multi-document input so a second
// document can never hide content from validation.
func DecodeYAML(data []byte) (any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "parse YAML", err)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, aerr.New(aerr.StateSchema, "parse YAML: more than one document")
	}
	return doc, nil
}

// DetectID returns the value of the document's top-level "schema" field.
func DetectID(doc any) (string, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return "", aerr.New(aerr.StateSchema, "document is not a mapping, so it has no schema field")
	}
	id, ok := m["schema"].(string)
	if !ok || id == "" {
		return "", aerr.New(aerr.StateSchema, "document has no string schema field")
	}
	return id, nil
}

// toJSONValue converts decoded YAML (or Go values) to the plain JSON value model the
// validator expects. The JSON round trip turns timestamps into strings and rejects
// non-string mapping keys.
func toJSONValue(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

// ViolationErrors lists every schema violation as "<instance location>: <message>", sorted.
type ViolationErrors []string

func (p ViolationErrors) Error() string { return strings.Join(p, "; ") }

func problemsError(id string, err error) error {
	ve, ok := err.(*jsonschema.ValidationError) //nolint:errorlint // Validate returns this type directly, never wrapped
	if !ok {
		return aerr.Wrap(aerr.StateSchema, "validate against "+id, err)
	}
	var list ViolationErrors
	collect(ve.BasicOutput(), &list)
	slices.Sort(list)
	list = slices.Compact(list)
	return aerr.Wrap(aerr.StateSchema, fmt.Sprintf("document does not match %s (%d violations)", id, len(list)), list)
}

func collect(u *jsonschema.OutputUnit, list *ViolationErrors) {
	if u.Error != nil {
		loc := u.InstanceLocation
		if loc == "" {
			loc = "/"
		}
		*list = append(*list, loc+": "+u.Error.String())
	}
	for i := range u.Errors {
		collect(&u.Errors[i], list)
	}
}
