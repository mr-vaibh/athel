package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	athel "github.com/mr-vaibh/athel"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Layer names, in order of authority.
const (
	LayerBuiltin = "builtin"
	LayerOrg     = "org"
	LayerUser    = "user"
	LayerProject = "project"
)

// AthelRules are the merge rules for athel.yaml (docs/plans/M0-plan.md §7).
var AthelRules = Rules{
	"schema":                         {Kind: Free},
	"budgets.default.max_cost_usd":   {Kind: Min},
	"budgets.default.max_hours":      {Kind: Min},
	"budgets.default.max_tool_calls": {Kind: Min},
	"budgets.warn_ratio":             {Kind: Min}, // warning earlier is stricter
	"locks.ttl_min":                  {Kind: Max}, // a longer TTL protects the writer longer
	"locks.heartbeat_min":            {Kind: Min}, // more frequent heartbeats are stricter
	"identity":                       {Kind: Free},
	"features.*":                     {Kind: Free},
	// M2 (config@2). Search roots widen rule 12, so lower layers may only remove roots.
	"repos.search_roots":                       {Kind: Intersect},
	"facts.freshness.*":                        {Kind: Min}, // a shorter window re-verifies sooner
	"facts.archive_after_days":                 {Kind: Free},
	"discovery.lookback_hours":                 {Kind: Max}, // a longer lookback misses fewer callers
	"discovery.question_budget.max_tool_calls": {Kind: Min},
	"discovery.question_budget.max_minutes":    {Kind: Min},
	"models.scout":                             {Kind: Free},
}

// reservedFiles are policy files later milestones define; they are not read yet.
var reservedFiles = []string{"gates.yaml", "services.yaml"}

// Paths locates the on-disk layers. Home is ATHEL_HOME (normally ~/.athel); tests always pass
// a temporary directory so the real home folder is never read.
type Paths struct {
	Home       string
	ProjectDir string // .athel/config of the workspace; empty outside a workspace
}

// DefaultHome resolves ATHEL_HOME, falling back to ~/.athel.
func DefaultHome(getenv func(string) string, userHome func() (string, error)) (string, error) {
	if h := getenv("ATHEL_HOME"); h != "" {
		return h, nil
	}
	home, err := userHome()
	if err != nil {
		return "", aerr.Wrap(aerr.StateIO, "find the home directory (set ATHEL_HOME)", err)
	}
	return filepath.Join(home, ".athel"), nil
}

// Result is the outcome of loading every layer.
type Result struct {
	Effective *Effective
	Layers    []Layer
	Hash      string   // sha256 of the canonical JSON of the effective values
	Notes     []string // informational, e.g. reserved files present but not read in M0
}

// FileSpec describes one layered policy or config file.
type FileSpec struct {
	Name      string // file name in the org and project layers, e.g. "tools.yaml"
	Schema    string
	Rules     Rules
	UserFile  string // file in ATHEL_HOME for the user layer; "" when the file has no user layer
	Normalize func(map[string]any) (map[string]any, error)
	// Legacy lists older schema versions still read. Each must be a strict subset of Schema, so
	// a layer in an older version is validated against it and then treated as current.
	Legacy []string
}

// AthelSpec is athel.yaml.
var AthelSpec = FileSpec{Name: "athel.yaml", Schema: schema.Config, Rules: AthelRules, UserFile: "user.yaml", Legacy: []string{schema.ConfigV1}}

// LoadFile reads every present layer of one file (built-in from the binary, then org, user,
// project), validates each against its schema, normalises it and merges them tighten-only.
func LoadFile(p Paths, reg *schema.Registry, spec FileSpec) (*Effective, []Layer, error) {
	builtinPath := "plugin/policies/defaults/" + spec.Name
	builtin, err := readLayer(reg, spec, LayerBuiltin, "builtin:"+builtinPath,
		func() ([]byte, error) { return fs.ReadFile(athel.Defaults, builtinPath) })
	if err != nil {
		return nil, nil, err
	}
	layers := []Layer{*builtin}
	candidates := []struct{ name, path string }{{LayerOrg, filepath.Join(p.Home, "org", spec.Name)}}
	if spec.UserFile != "" {
		candidates = append(candidates, struct{ name, path string }{LayerUser, filepath.Join(p.Home, spec.UserFile)})
	}
	if p.ProjectDir != "" {
		candidates = append(candidates, struct{ name, path string }{LayerProject, filepath.Join(p.ProjectDir, spec.Name)})
	}
	for _, c := range candidates {
		l, err := readLayer(reg, spec, c.name, c.path, func() ([]byte, error) { return os.ReadFile(c.path) })
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		layers = append(layers, *l)
	}
	eff, err := Merge(layers, spec.Rules)
	if err != nil {
		return nil, nil, err
	}
	return eff, layers, nil
}

// Load reads athel.yaml from every present layer and computes the effective hash.
func Load(p Paths, reg *schema.Registry) (*Result, error) {
	eff, layers, err := LoadFile(p, reg, AthelSpec)
	if err != nil {
		return nil, err
	}
	hash, err := digest.CanonicalJSON(eff.Values)
	if err != nil {
		return nil, err
	}
	return &Result{Effective: eff, Layers: layers, Hash: hash, Notes: reservedNotes(p.ProjectDir)}, nil
}

func readLayer(reg *schema.Registry, spec FileSpec, name, path string, read func() ([]byte, error)) (*Layer, error) {
	data, err := read()
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read config "+path, err)
	}
	doc, err := schema.DecodeYAML(data)
	if err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, name+" "+spec.Name+" "+path+" is not valid YAML", err)
	}
	m, _ := doc.(map[string]any)
	id := spec.Schema
	if v, _ := m["schema"].(string); slices.Contains(spec.Legacy, v) {
		id = v
	}
	if err := reg.Validate(id, doc); err != nil {
		return nil, aerr.Wrap(aerr.PolicyConfigInvalid, name+" "+spec.Name+" "+path+" does not match "+id, err)
	}
	m["schema"] = spec.Schema // the schema guarantees an object; a legacy layer is upgraded in memory
	if spec.Normalize != nil {
		if m, err = spec.Normalize(m); err != nil {
			return nil, aerr.Wrap(aerr.PolicyConfigInvalid, name+" "+spec.Name+" "+path, err)
		}
	}
	return &Layer{Name: name, Path: path, Doc: m}, nil
}

func reservedNotes(projectDir string) []string {
	notes := []string{}
	if projectDir == "" {
		return notes
	}
	for _, f := range reservedFiles {
		if _, err := os.Stat(filepath.Join(projectDir, f)); err == nil {
			notes = append(notes, f+" is reserved for a later milestone and is not read yet")
		}
	}
	return notes
}
