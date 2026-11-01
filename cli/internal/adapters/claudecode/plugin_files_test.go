package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

type hookHandler struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

type hookGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookHandler `json:"hooks"`
}

// pluginDir is the plugin folder relative to this package.
var pluginDir = filepath.Join("..", "..", "..", "..", "plugin")

func TestHooksJSONCoversDocEvents(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Hooks) != len(hookio.Events) {
		t.Errorf("hooks.json wires %d events, doc 02 lists %d", len(cfg.Hooks), len(hookio.Events))
	}
	toolEvents := []hookio.Event{hookio.PreToolUse, hookio.PostToolUse, hookio.PostToolUseFailure}
	for _, e := range hookio.Events {
		groups := cfg.Hooks[string(e)]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Errorf("%s: want exactly one handler, got %+v", e, groups)
			continue
		}
		h := groups[0].Hooks[0]
		want := []string{"hook", CLIName(e)}
		if h.Type != "command" || h.Command != "${CLAUDE_PLUGIN_ROOT}/bin/athel" || !slices.Equal(h.Args, want) {
			t.Errorf("%s handler = %+v, want athel %v", e, h, want)
		}
		if h.Timeout < 1 || h.Timeout > 10 {
			t.Errorf("%s timeout %d s: hooks must be bounded (Claude Code defaults to 600 s)", e, h.Timeout)
		}
		if slices.Contains(toolEvents, e) != (groups[0].Matcher == "*") {
			t.Errorf("%s matcher %q: tool events match every tool, others take no matcher", e, groups[0].Matcher)
		}
	}
}

type skill struct {
	frontmatter map[string]any
	body        string
}

func readSkill(t *testing.T, name string) skill {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(pluginDir, "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(b), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatalf("%s: SKILL.md has no frontmatter", name)
	}
	var fm map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &fm); err != nil {
		t.Fatalf("%s frontmatter: %v", name, err)
	}
	return skill{frontmatter: fm, body: parts[2]}
}

// mutatingCommands are athel commands that change state; skills that run them must pass the
// session so the CLI can take the change lock for it.
var mutatingCommands = regexp.MustCompile(`athel [^\n` + "`" + `]*\b(change new|state (transition|enter|exit)|checkpoint set|lock (acquire|release|refresh)|budget add|resume)\b`)

func TestSkillsCallOnlyCLI(t *testing.T) {
	for _, name := range []string{"start", "status", "resume"} {
		t.Run(name, func(t *testing.T) {
			s := readSkill(t, name)
			if d, _ := s.frontmatter["description"].(string); len(d) < 20 {
				t.Errorf("description too short: %q", d)
			}
			if s.frontmatter["allowed-tools"] != "Bash(athel:*)" {
				t.Errorf("allowed-tools = %v: skills may only run the athel CLI", s.frontmatter["allowed-tools"])
			}
			if !strings.Contains(s.body, ".athel/") || !strings.Contains(s.body, "Never") {
				t.Error("skill does not forbid editing .athel/ directly")
			}
			for _, m := range mutatingCommands.FindAllString(s.body, -1) {
				if !strings.Contains(m, "--session ${CLAUDE_SESSION_ID}") {
					t.Errorf("mutating command without the session: %q", m)
				}
			}
			for _, line := range strings.Split(s.body, "\n") {
				if strings.Contains(line, "athel ") && strings.Contains(line, "`athel") && !strings.Contains(line, "--json") {
					t.Errorf("athel command without --json: %q", line)
				}
			}
		})
	}
	if mutatingCommands.MatchString(readSkill(t, "status").body) {
		t.Error("/athel:status must be read-only")
	}
	if readSkill(t, "start").frontmatter["disable-model-invocation"] != true {
		t.Error("/athel:start creates changes and must only run when the user asks")
	}
}
