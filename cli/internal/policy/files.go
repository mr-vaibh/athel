// Package policy loads Athel's policy files (tools.yaml, environments.yaml, git.yaml) and
// decides every tool call (doc 04 §Permission tiers and the policy decision).
package policy

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/mr-vaibh/athel/cli/internal/config"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Decision strictness: lower layers may only move a value right.
var decisionOrder = []string{"allow", "approve", "deny"}

// ToolsSpec is tools.yaml. The file is a list of entries keyed by id; it is merged as a map so
// layers can refine one tool. Fixed rules (prod writes, W3, tier X) never consult it.
var ToolsSpec = config.FileSpec{
	Name: "tools.yaml", Schema: "athel/tools@1", Normalize: toolsByID,
	Rules: config.Rules{
		"schema":                           {Kind: config.Free},
		"tools.*.id":                       {Kind: config.Free},
		"tools.*.kind":                     {Kind: config.Free},
		"tools.*.match.command":            {Kind: config.Free},
		"tools.*.match.server":             {Kind: config.Free},
		"tools.*.match.tools":              {Kind: config.Free},
		"tools.*.purpose":                  {Kind: config.Free},
		"tools.*.read":                     {Kind: config.Intersect}, // fewer read verbs is stricter
		"tools.*.write":                    {Kind: config.Union},     // more verbs counted as writes is stricter
		"tools.*.environments.detect.flag": {Kind: config.Free},
		"tools.*.environments.detect.default_from": {Kind: config.Free},
		"tools.*.policy.*.read":                    {Kind: config.Ordered, Order: decisionOrder},
		"tools.*.policy.*.write":                   {Kind: config.Ordered, Order: decisionOrder},
		"tools.*.forbidden_patterns":               {Kind: config.Union},
		"tools.*.playbook":                         {Kind: config.Free},
		"tools.*.auth_check":                       {Kind: config.Free},
	},
}

// EnvironmentsSpec is environments.yaml. Patterns that make a target prod may only be added;
// patterns for any other environment may only be removed below the layer that defined them.
var EnvironmentsSpec = config.FileSpec{
	Name: "environments.yaml", Schema: "athel/environments@1",
	Rules: config.Rules{
		"schema":              {Kind: config.Free},
		"environments.prod.*": {Kind: config.Union},
		"environments.*.*":    {Kind: config.Intersect},
		"default":             {Kind: config.Ordered, Order: []string{"dev", "staging", "prod"}},
	},
}

// GitSpec is git.yaml (doc 04 §Git policy).
var GitSpec = config.FileSpec{
	Name: "git.yaml", Schema: "athel/git@1",
	Rules: config.Rules{
		"schema":                      {Kind: config.Free},
		"host":                        {Kind: config.Free},
		"base_branch":                 {Kind: config.Free},
		"protected":                   {Kind: config.Union},
		"branch_name":                 {Kind: config.Free},
		"side_finding_branch":         {Kind: config.Free},
		"commit.format":               {Kind: config.Ordered, Order: []string{"free", "conventional"}},
		"commit.trailer":              {Kind: config.Union},
		"commit.max_files_per_commit": {Kind: config.Min},
		"commit.sign":                 {Kind: config.Free},
		"pr.draft_until_gates_pass":   {Kind: config.Ordered, Order: []string{"false", "true"}},
		"pr.max_changed_lines":        {Kind: config.Min},
		"pr.exclude_from_size":        {Kind: config.Intersect},
		"pr.template":                 {Kind: config.Free},
		"pr.labels":                   {Kind: config.Union},
		"pr.request_reviewers_from":   {Kind: config.Free},
		"forbidden":                   {Kind: config.Union},
	},
}

func toolsByID(doc map[string]any) (map[string]any, error) {
	list, _ := doc["tools"].([]any)
	byID := map[string]any{}
	for _, item := range list {
		entry, _ := item.(map[string]any) // the schema guarantees objects with an id
		id, _ := entry["id"].(string)
		if _, dup := byID[id]; dup {
			return nil, fmt.Errorf("tool %q is listed twice", id)
		}
		byID[id] = entry
	}
	out := map[string]any{}
	for k, v := range doc {
		out[k] = v
	}
	out["tools"] = byID
	return out, nil
}

// Tool is one tools.yaml entry after merging.
type Tool struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Match struct {
		Command string   `json:"command"`
		Server  string   `json:"server"`
		Tools   []string `json:"tools"`
	} `json:"match"`
	Purpose      []string `json:"purpose"`
	Read         []string `json:"read"`
	Write        []string `json:"write"`
	Environments struct {
		Detect struct {
			Flag        string `json:"flag"`
			DefaultFrom string `json:"default_from"`
		} `json:"detect"`
	} `json:"environments"`
	Policy            map[string]EnvPolicy `json:"policy"`
	ForbiddenPatterns []string             `json:"forbidden_patterns"`
	Playbook          string               `json:"playbook"`
	AuthCheck         string               `json:"auth_check"`

	forbidden []*regexp.Regexp
}

// EnvPolicy is a tool's read and write decision in one environment.
type EnvPolicy struct {
	Read  string `json:"read"`
	Write string `json:"write"`
}

// Environment is one environments.yaml entry.
type Environment struct {
	KubeContexts   []string `json:"kube_contexts"`
	GcloudProjects []string `json:"gcloud_projects"`
	AWSAccounts    []string `json:"aws_accounts"`
	URLs           []string `json:"urls"`
}

// Git is the subset of git.yaml the policy engine enforces in M1 (plus the rest for later).
type Git struct {
	BaseBranch        string    `json:"base_branch"`
	Protected         []string  `json:"protected"`
	Forbidden         []string  `json:"forbidden"`
	BranchName        string    `json:"branch_name"`         // M4: the template for Athel's branches
	SideFindingBranch string    `json:"side_finding_branch"` // M4: the template for side-finding fix branches
	Commit            GitCommit `json:"commit"`              // M4: how Athel's commits look (FR-GIT-005)
	PR                GitPR     `json:"pr"`                  // M4: Athel's pull requests (FR-GIT-007/009)
}

// GitPR is git.yaml's pr section.
type GitPR struct {
	DraftUntilGatesPass bool     `json:"draft_until_gates_pass"`
	MaxChangedLines     int      `json:"max_changed_lines"`
	ExcludeFromSize     []string `json:"exclude_from_size"`
}

// GitCommit is git.yaml's commit section.
type GitCommit struct {
	Format   string   `json:"format"`
	Trailer  []string `json:"trailer"`
	MaxFiles int      `json:"max_files_per_commit"`
	Sign     string   `json:"sign"`
}

// Set is the effective policy: every file merged, typed and hashed.
type Set struct {
	Tools        map[string]*Tool          `json:"tools"`
	Environments map[string]Environment    `json:"environments"`
	DefaultEnv   string                    `json:"default"`
	Git          Git                       `json:"git"`
	Hash         string                    `json:"hash"`
	Layers       map[string][]config.Layer `json:"-"`
	Values       map[string]map[string]any `json:"-"`
}

// Load reads and merges every policy file. Any problem is an E-POLICY error: an invalid policy
// disables all writes (FR-SAFE-012), which the engine enforces as rule 0.
func Load(p config.Paths, reg *schema.Registry, athelHash string) (*Set, error) {
	s := &Set{Layers: map[string][]config.Layer{}, Values: map[string]map[string]any{}}
	for _, spec := range []config.FileSpec{ToolsSpec, EnvironmentsSpec, GitSpec} {
		eff, layers, err := config.LoadFile(p, reg, spec)
		if err != nil {
			return nil, err
		}
		s.Layers[spec.Name], s.Values[spec.Name] = layers, eff.Values
	}
	if err := decode(s.Values["tools.yaml"]["tools"], &s.Tools); err != nil {
		return nil, err
	}
	var envs struct {
		Environments map[string]Environment `json:"environments"`
		Default      string                 `json:"default"`
	}
	if err := decode(s.Values["environments.yaml"], &envs); err != nil {
		return nil, err
	}
	s.Environments, s.DefaultEnv = envs.Environments, envs.Default
	if err := decode(s.Values["git.yaml"], &s.Git); err != nil {
		return nil, err
	}
	if err := s.compile(); err != nil {
		return nil, err
	}
	hash, err := digest.CanonicalJSON(map[string]any{"athel": athelHash, "files": s.Values})
	if err != nil {
		return nil, err
	}
	s.Hash = hash
	return s, nil
}

func decode(v any, into any) error {
	b, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(b, into)
	}
	if err != nil {
		return aerr.Wrap(aerr.PolicyConfigInvalid, "decode effective policy", err)
	}
	return nil
}

// compile checks every forbidden pattern once, so a broken regex is a load error, never a
// silent non-match at decision time.
func (s *Set) compile() error {
	for id, t := range s.Tools {
		t.ID = id
		for _, p := range t.ForbiddenPatterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return aerr.Wrap(aerr.PolicyConfigInvalid, fmt.Sprintf("tools.yaml: tool %s has an invalid forbidden pattern %q", id, p), err)
			}
			t.forbidden = append(t.forbidden, re)
		}
	}
	return nil
}

// ForbiddenRegexps returns the tool's compiled forbidden patterns.
func (t *Tool) ForbiddenRegexps() []*regexp.Regexp { return t.forbidden }
