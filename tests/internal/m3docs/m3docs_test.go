package m3docs

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestDocsParse(t *testing.T) {
	var plan struct {
		Schema string `yaml:"schema"`
		Tasks  []struct {
			Repo  string   `yaml:"repo"`
			Kind  string   `yaml:"kind"`
			Paths []string `yaml:"paths"`
			Verbs []string `yaml:"verbs"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal([]byte(Plan("svc", []string{"src/**", "test/**"}, []string{"edit", "test"})), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Schema != "athel/plan@2" || plan.Tasks[0].Repo != "svc" || plan.Tasks[0].Kind != "internal" || len(plan.Tasks[0].Paths) != 2 || plan.Tasks[0].Verbs[1] != "test" {
		t.Errorf("plan: %+v", plan)
	}
	for name, doc := range map[string]string{"review": Review, "spec": Spec, "conventions": Conventions} {
		var m map[string]any
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil || len(m) == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestShellMirrorsGo keeps m3-prepare.sh's documents equal to the Go ones.
func TestShellMirrorsGo(t *testing.T) {
	b, err := os.ReadFile("m3-prepare.sh")
	if err != nil {
		t.Fatal(err)
	}
	sh := string(b)
	for name, doc := range map[string]string{"review": Review, "spec": Spec, "conventions": Conventions,
		"plan": Plan("svc", []string{"src/**"}, []string{"edit", "test"})} {
		if !strings.Contains(sh, doc) {
			t.Errorf("m3-prepare.sh's %s differs from m3docs.go", name)
		}
	}
}

func TestPreflightReport(t *testing.T) {
	md := Preflight("svc", []string{"src/**", "test/**"})
	_, block, _ := strings.Cut(md, "```yaml\n")
	block, _, _ = strings.Cut(block, "```")
	var r struct {
		Schema   string   `yaml:"schema"`
		Repo     string   `yaml:"repo"`
		BaseHead string   `yaml:"base_head"`
		Paths    []string `yaml:"planned_paths"`
		Clean    bool     `yaml:"clean"`
	}
	if err := yaml.Unmarshal([]byte(block), &r); err != nil {
		t.Fatal(err)
	}
	if r.Schema != "athel/preflight@1" || r.Repo != "svc" || len(r.BaseHead) != 40 || strings.Join(r.Paths, ",") != "src/**,test/**" || !r.Clean {
		t.Errorf("%+v", r)
	}
}
