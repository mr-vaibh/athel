package athel_test

import (
	"encoding/json"
	"regexp"
	"testing"

	athel "github.com/mr-vaibh/athel"
)

func TestPluginManifest(t *testing.T) {
	var m struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		Metadata struct {
			Athel struct {
				MinClaudeCode string `json:"min_claude_code_version"`
			} `json:"athel"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(athel.PluginManifest, &m); err != nil {
		t.Fatal(err)
	}
	semver := regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	if m.Name != "athel" || !semver.MatchString(m.Version) || !semver.MatchString(m.Metadata.Athel.MinClaudeCode) {
		t.Errorf("manifest = %+v", m)
	}
}
