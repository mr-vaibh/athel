package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestMigrateConfig1To2(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain, comments kept", "# team settings\nschema: athel/config@1 # version\nidentity: dev # me\n",
			"# team settings\nschema: athel/config@2 # version\nidentity: dev # me\n"},
		{"double quoted", "schema: \"athel/config@1\"\n", "schema: \"athel/config@2\"\n"},
		{"single quoted, spaces", "schema:   'athel/config@1'  \nlocks:\n  ttl_min: 45\n", "schema:   'athel/config@2'  \nlocks:\n  ttl_min: 45\n"},
		{"crlf", "schema: athel/config@1\r\nidentity: dev\r\n", "schema: athel/config@2\r\nidentity: dev\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "user.yaml")
			write(t, path, tt.in)
			reg := schema.NewRegistry()
			got, err := MigrateLayers(Paths{Home: home}, reg, true)
			if err != nil || !slices.Equal(got, []string{path}) {
				t.Fatalf("dry run = %v %v", got, err)
			}
			if b, _ := os.ReadFile(path); string(b) != tt.in {
				t.Fatal("dry run wrote the file")
			}
			if got, err := MigrateLayers(Paths{Home: home}, reg, false); err != nil || len(got) != 1 {
				t.Fatalf("migrate = %v %v", got, err)
			}
			if b, _ := os.ReadFile(path); string(b) != tt.want {
				t.Errorf("migrated file = %q, want %q", b, tt.want)
			}
			if got, err := MigrateLayers(Paths{Home: home}, reg, false); err != nil || len(got) != 0 {
				t.Errorf("second migrate = %v %v", got, err)
			}
			if _, err := Load(Paths{Home: home}, reg); err != nil {
				t.Errorf("load after migrate: %v", err)
			}
		})
	}
}

func TestMigrateConfigAllLayers(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	org := filepath.Join(home, "org", "athel.yaml")
	write(t, org, "schema: athel/config@1\n")
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@2\n")
	write(t, filepath.Join(project, "athel.yaml"), "schema: athel/config@1\n")
	got, err := MigrateLayers(Paths{Home: home, ProjectDir: project}, schema.NewRegistry(), false)
	if err != nil || !slices.Equal(got, []string{org, filepath.Join(project, "athel.yaml")}) {
		t.Errorf("migrated = %v %v", got, err)
	}
}

func TestMigrateConfigErrors(t *testing.T) {
	tests := []struct {
		name, in string
		code     aerr.Code
	}{
		{"flow style", "{schema: athel/config@1, identity: dev}\n", aerr.PolicyConfigInvalid},
		{"not yaml", "a: [", aerr.PolicyConfigInvalid},
		{"invalid content", "schema: athel/config@1\nbogus: 1\n", aerr.PolicyConfigInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			write(t, filepath.Join(home, "user.yaml"), tt.in)
			if _, err := MigrateLayers(Paths{Home: home}, schema.NewRegistry(), false); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
	home := t.TempDir()
	write(t, filepath.Join(home, "user.yaml"), "- a list\n")
	if got, err := MigrateLayers(Paths{Home: home}, schema.NewRegistry(), false); err != nil || len(got) != 0 {
		t.Errorf("non-mapping document = %v %v", got, err)
	}
	unreadable := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unreadable, "user.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLayers(Paths{Home: unreadable}, schema.NewRegistry(), false); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable layer: %v, want E-STATE-IO", err)
	}
}

func TestLegacyLayerCannotUseNewKeys(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@1\nmodels:\n  scout: opus\n")
	if _, err := Load(Paths{Home: home}, schema.NewRegistry()); !aerr.HasCode(err, aerr.PolicyConfigInvalid) {
		t.Errorf("config@1 layer with an M2 key: %v, want E-POLICY-CONFIG-INVALID", err)
	}
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@2\nmodels:\n  scout: opus\ndiscovery:\n  lookback_hours: 24\n")
	if _, err := Load(Paths{Home: home}, schema.NewRegistry()); !aerr.HasCode(err, aerr.PolicyConfigLoosens) {
		t.Errorf("shorter lookback than the built-in 168 h: %v, want E-POLICY-CONFIG-LOOSENS", err)
	}
	write(t, filepath.Join(home, "user.yaml"), "schema: athel/config@2\nmodels:\n  scout: opus\nrepos:\n  search_roots: [~/Developer]\nfacts:\n  freshness:\n    topology: 60\n")
	r, err := Load(Paths{Home: home}, schema.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if r.Effective.Origins["repos.search_roots"] != LayerUser || r.Effective.Origins["models.scout"] != LayerUser {
		t.Errorf("origins = %v", r.Effective.Origins)
	}
}
