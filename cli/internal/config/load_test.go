package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinFactSettings(t *testing.T) {
	r, err := Load(Paths{Home: t.TempDir()}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	s := r.Settings()
	day := 24 * time.Hour
	want := map[string]time.Duration{"volatile": 0, "runtime": time.Hour, "topology": day, "structural": 7 * day, "organisational": 7 * day}
	for class, w := range want {
		if got, ok := s.FactWindows[class]; !ok || got != w {
			t.Errorf("window %s = %v, want %v", class, got, w)
		}
	}
	if s.FactArchiveAfter != 90*day {
		t.Errorf("archive after = %v", s.FactArchiveAfter)
	}
}

func TestBuiltinOnly(t *testing.T) {
	r, err := Load(Paths{Home: t.TempDir()}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	s := r.Settings()
	if s.BudgetMaxCostUSD != 40 || s.BudgetMaxHours != 8 || s.BudgetMaxToolCalls != 2000 || s.WarnRatio != 0.8 ||
		s.LockTTLMin != 30 || s.HeartbeatEvery != 5*time.Minute || s.Identity != "" ||
		len(s.Features) != 1 || s.Features["discovery"] {
		t.Errorf("settings = %+v", s)
	}
	if len(r.Layers) != 1 || r.Layers[0].Name != LayerBuiltin || !strings.HasPrefix(r.Hash, "sha256:") {
		t.Errorf("layers %v hash %s", r.Layers, r.Hash)
	}
	if len(r.Notes) != 0 {
		t.Errorf("notes = %v", r.Notes)
	}
}

func TestLayersFromTempDirs(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	write(t, filepath.Join(home, "org", "athel.yaml"), "schema: athel/config@1\noverridable: [identity]\nbudgets:\n  default:\n    max_cost_usd: 30\n")
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@1\nidentity: dev@example.com\nfeatures:\n  m1_policy: true\n")
	write(t, filepath.Join(project, "athel.yaml"), "schema: athel/config@1\nbudgets:\n  default:\n    max_cost_usd: 25\n")
	write(t, filepath.Join(project, "gates.yaml"), "anything")
	r, err := Load(Paths{Home: home, ProjectDir: project}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	s := r.Settings()
	if s.BudgetMaxCostUSD != 25 || s.Identity != "dev@example.com" || !s.Features["m1_policy"] {
		t.Errorf("settings = %+v", s)
	}
	if r.Effective.Origins["budgets.default.max_cost_usd"] != LayerProject || r.Effective.Origins["budgets.default.max_hours"] != LayerBuiltin {
		t.Errorf("origins = %v", r.Effective.Origins)
	}
	if len(r.Layers) != 4 || len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "gates.yaml") {
		t.Errorf("layers %d notes %v", len(r.Layers), r.Notes)
	}
}

func TestMissingLayersOK(t *testing.T) {
	if _, err := Load(Paths{Home: filepath.Join(t.TempDir(), "nope"), ProjectDir: filepath.Join(t.TempDir(), "nope")}, schema.NewRegistry()); err != nil {
		t.Errorf("missing layers: %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		project string
		code    aerr.Code
	}{
		{"loosens builtin", "schema: athel/config@1\nbudgets:\n  default:\n    max_cost_usd: 100\n", aerr.PolicyConfigLoosens},
		{"schema violation", "schema: athel/config@1\nbudget: 1\n", aerr.PolicyConfigInvalid},
		{"wrong schema id", "schema: athel/config@9\n", aerr.PolicyConfigInvalid},
		{"not yaml", "a: [", aerr.PolicyConfigInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			write(t, filepath.Join(project, "athel.yaml"), tt.project)
			if _, err := Load(Paths{Home: t.TempDir(), ProjectDir: project}, schema.NewRegistry()); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	unreadable := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unreadable, "athel.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Paths{Home: t.TempDir(), ProjectDir: unreadable}, schema.NewRegistry()); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("athel.yaml is a directory: err = %v", err)
	}
}

func TestEffectiveHashStable(t *testing.T) {
	home := t.TempDir()
	a, err := Load(Paths{Home: home}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(Paths{Home: home}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if a.Hash != b.Hash {
		t.Errorf("hash changed between identical loads: %s vs %s", a.Hash, b.Hash)
	}
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@1\nidentity: someone\n")
	c, err := Load(Paths{Home: home}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if c.Hash == a.Hash {
		t.Error("hash did not change when a value changed")
	}
}

func TestNeverReadsRealHome(t *testing.T) {
	poisoned := t.TempDir()
	write(t, filepath.Join(poisoned, ".athel", "user.yaml"), "this is not valid config")
	t.Setenv("HOME", poisoned)
	if _, err := Load(Paths{Home: t.TempDir()}, schema.NewRegistry()); err != nil {
		t.Errorf("Load read $HOME instead of the injected path: %v", err)
	}
}

func TestDefaultHome(t *testing.T) {
	env := map[string]string{"ATHEL_HOME": "/custom"}
	getenv := func(k string) string { return env[k] }
	home := func() (string, error) { return "/home/dev", nil }
	if got, err := DefaultHome(getenv, home); err != nil || got != "/custom" {
		t.Errorf("with ATHEL_HOME: %s %v", got, err)
	}
	delete(env, "ATHEL_HOME")
	if got, err := DefaultHome(getenv, home); err != nil || got != filepath.Join("/home/dev", ".athel") {
		t.Errorf("fallback: %s %v", got, err)
	}
	broken := func() (string, error) { return "", errors.New("no home") }
	if _, err := DefaultHome(getenv, broken); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("no home: err = %v", err)
	}
}
