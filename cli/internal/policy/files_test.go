package policy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/config"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func load(t *testing.T, project map[string]string, org map[string]string) (*Set, error) {
	t.Helper()
	home, proj := t.TempDir(), t.TempDir()
	for name, content := range org {
		if err := os.MkdirAll(filepath.Join(home, "org"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "org", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range project {
		if err := os.WriteFile(filepath.Join(proj, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Load(config.Paths{Home: home, ProjectDir: proj}, schema.NewRegistry(), "sha256:athel")
}

func TestBuiltinPoliciesValid(t *testing.T) {
	s, err := load(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"git", "gh", "kubectl", "gcloud", "aws", "praxis", "raptor"} {
		if s.Tools[id] == nil || s.Tools[id].ID != id {
			t.Errorf("built-in tool %s missing", id)
		}
	}
	if s.DefaultEnv != "prod" || len(s.Environments) != 0 {
		t.Errorf("environments = %v default %s", s.Environments, s.DefaultEnv)
	}
	if s.Git.BaseBranch != "main" || !slices.Contains(s.Git.Protected, "release/*") || !slices.Contains(s.Git.Forbidden, "force_push") {
		t.Errorf("git = %+v", s.Git)
	}
	if len(s.Tools["kubectl"].ForbiddenRegexps()) != 3 || len(s.Tools["raptor"].ForbiddenRegexps()) != 0 {
		t.Errorf("compiled patterns: %d", len(s.Tools["kubectl"].ForbiddenRegexps()))
	}
	if s.Tools["raptor"].Policy["prod"].Write != "deny" || len(s.Tools["kubectl"].forbidden) != 3 || !strings.HasPrefix(s.Hash, "sha256:") {
		t.Errorf("tools = %+v", s.Tools["raptor"])
	}
}

func TestPolicyMergeTightenOnly(t *testing.T) {
	const tools, envs, git = "schema: athel/tools@1\n", "schema: athel/environments@1\n", "schema: athel/git@1\n"
	tests := []struct {
		name    string
		org     map[string]string
		project map[string]string
		code    aerr.Code
	}{
		{"remove a read verb", nil, map[string]string{"tools.yaml": tools + "tools:\n  - {id: kubectl, kind: cli, match: {command: kubectl}, read: [get]}\n"}, ""},
		{"add a read verb", nil, map[string]string{"tools.yaml": tools + "tools:\n  - {id: kubectl, kind: cli, match: {command: kubectl}, read: [get, exec]}\n"}, aerr.PolicyConfigLoosens},
		{"loosen a policy", nil, map[string]string{"tools.yaml": tools + "tools:\n  - {id: kubectl, kind: cli, match: {command: kubectl}, policy: {staging: {write: allow}}}\n"}, aerr.PolicyConfigLoosens},
		{"register a new MCP tool", nil, map[string]string{"tools.yaml": tools + "tools:\n  - {id: grafana, kind: mcp, match: {server: grafana, tools: ['*']}, read: [query], policy: {prod: {read: allow, write: deny}}}\n"}, ""},
		{"default to staging", nil, map[string]string{"environments.yaml": envs + "default: staging\n"}, aerr.PolicyConfigLoosens},
		{"add prod patterns", map[string]string{"environments.yaml": envs + "environments:\n  prod: {kube_contexts: ['gke_acme-prod_*']}\n"},
			map[string]string{"environments.yaml": envs + "environments:\n  prod: {kube_contexts: ['*-prod-*']}\n"}, ""},
		{"widen staging patterns", map[string]string{"environments.yaml": envs + "environments:\n  staging: {kube_contexts: ['gke_acme-staging_*']}\n"},
			map[string]string{"environments.yaml": envs + "environments:\n  staging: {kube_contexts: ['gke_acme-staging_*', '*']}\n"}, aerr.PolicyConfigLoosens},
		{"unprotect main", nil, map[string]string{"git.yaml": git + "protected: [develop]\n"}, ""},
		{"bigger PRs", nil, map[string]string{"git.yaml": git + "pr: {max_changed_lines: 800}\n"}, aerr.PolicyConfigLoosens},
		{"drop forbidden", nil, map[string]string{"git.yaml": git + "forbidden: [merge]\n"}, ""},
		{"draft off", nil, map[string]string{"git.yaml": git + "pr: {draft_until_gates_pass: false}\n"}, aerr.PolicyConfigLoosens},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := load(t, tt.project, tt.org)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
			_ = s
		})
	}
	s, _ := load(t, map[string]string{"git.yaml": git + "protected: [develop]\nforbidden: [merge]\n"}, nil)
	if !slices.Contains(s.Git.Protected, "main") || !slices.Contains(s.Git.Protected, "develop") || !slices.Contains(s.Git.Forbidden, "force_push") {
		t.Errorf("union lists must keep built-in entries: %+v", s.Git)
	}
}

func TestInvalidPolicyFile(t *testing.T) {
	tests := []struct {
		name    string
		project map[string]string
	}{
		{"schema violation", map[string]string{"tools.yaml": "schema: athel/tools@1\ntools: [{id: x}]\n"}},
		{"duplicate id", map[string]string{"tools.yaml": "schema: athel/tools@1\ntools:\n  - {id: x, kind: cli, match: {command: x}}\n  - {id: x, kind: cli, match: {command: y}}\n"}},
		{"bad regex", map[string]string{"tools.yaml": "schema: athel/tools@1\ntools:\n  - {id: x, kind: cli, match: {command: x}, forbidden_patterns: ['(']}\n"}},
		{"bad env name", map[string]string{"environments.yaml": "schema: athel/environments@1\nenvironments: {Prod!: {}}\n"}},
		{"not yaml", map[string]string{"git.yaml": "a: ["}},
		{"unknown forbidden action", map[string]string{"git.yaml": "schema: athel/git@1\nforbidden: [anything]\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := load(t, tt.project, nil); !aerr.HasCode(err, aerr.PolicyConfigInvalid) {
				t.Errorf("err = %v, want E-POLICY-CONFIG-INVALID", err)
			}
		})
	}
}

func TestHashTracksContent(t *testing.T) {
	a, err := load(t, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := load(t, nil, nil)
	c, _ := load(t, map[string]string{"git.yaml": "schema: athel/git@1\nprotected: [develop]\n"}, nil)
	if a.Hash != b.Hash || a.Hash == c.Hash {
		t.Errorf("hashes: %s %s %s", a.Hash, b.Hash, c.Hash)
	}
	d, _ := Load(config.Paths{Home: t.TempDir()}, schema.NewRegistry(), "sha256:other")
	if d.Hash == a.Hash {
		t.Error("athel.yaml hash must be part of the policy hash")
	}
}

func TestDecodeError(t *testing.T) {
	var into map[string]int
	if err := decode(map[string]any{"a": "x"}, &into); !aerr.HasCode(err, aerr.PolicyConfigInvalid) {
		t.Errorf("err = %v", err)
	}
	if _, err := toolsByID(map[string]any{"tools": []any{map[string]any{"id": "a"}, map[string]any{"id": "a"}}}); err == nil {
		t.Error("duplicate ids accepted")
	}
}
