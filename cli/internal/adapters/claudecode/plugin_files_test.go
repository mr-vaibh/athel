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
	Type      string   `json:"type"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Timeout   int      `json:"timeout"`
	OnFailure string   `json:"onFailure"`
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
		// A PreToolUse hook that crashes or times out must block the call, not let it through
		// (code.claude.com/docs/en/hooks, "Block the action when a hook fails"; ADR 015).
		if (e == hookio.PreToolUse) != (h.OnFailure == "block") {
			t.Errorf("%s onFailure %q: only PreToolUse fails closed", e, h.OnFailure)
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
var mutatingCommands = regexp.MustCompile(`athel [^\n` + "`" + `]*\b(change new|state (transition|enter|exit)|checkpoint set|lock (acquire|release|refresh)|budget add|resume|decide (add|record|present|answer|tick)|blast (save|signoff)|spec save|plan set|trace|conventions save|prd (review save|add-version))\b`)

// checkSkillCommands: every athel command a skill names prints JSON, and every one that changes
// state carries the session so the CLI can take the change lock for it.
func checkSkillCommands(t *testing.T, s skill) {
	t.Helper()
	if d, _ := s.frontmatter["description"].(string); len(d) < 20 {
		t.Errorf("description too short: %q", d)
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
}

func TestSkillsCallOnlyCLI(t *testing.T) {
	for _, name := range []string{"start", "status", "resume", "approve", "accept", "decide", "signoff"} {
		t.Run(name, func(t *testing.T) {
			s := readSkill(t, name)
			if s.frontmatter["allowed-tools"] != "Bash(athel:*)" {
				t.Errorf("allowed-tools = %v: skills may only run the athel CLI", s.frontmatter["allowed-tools"])
			}
			checkSkillCommands(t, s)
		})
	}
	status := readSkill(t, "status").body
	if mutatingCommands.MatchString(status) {
		t.Error("/athel:status must be read-only")
	}
	if !strings.Contains(status, "athel --json decide list $ARGUMENTS --open") {
		t.Error("/athel:status must show the change's open decision cards")
	}
	if readSkill(t, "start").frontmatter["disable-model-invocation"] != true {
		t.Error("/athel:start creates changes and must only run when the user asks")
	}
}

// TestStepSkillsFollowTheStateMachine: the spec and plan skills delegate to the architect and move
// the change only along doc 02's transitions, ending each step before leaving its states.
func TestStepSkillsFollowTheStateMachine(t *testing.T) {
	steps := map[string][]string{
		"spec": {"state enter $ARGUMENTS spec", "--to specified --by athel", "decide present $ARGUMENTS --step spec",
			"state exit $ARGUMENTS spec --result ok", "--to awaiting_decisions --by athel"},
		"plan": {"decide tick $ARGUMENTS", "state enter $ARGUMENTS plan", "trace $ARGUMENTS", "--to planned --by athel",
			"state exit $ARGUMENTS plan --result ok", "plan summary $ARGUMENTS", "/athel:approve", "/athel:signoff"},
	}
	for name, order := range steps {
		t.Run(name, func(t *testing.T) {
			s := readSkill(t, name)
			checkSkillCommands(t, s)
			if s.frontmatter["allowed-tools"] != "Bash(athel:*), Agent" && s.frontmatter["allowed-tools"] != "Bash(athel:*), Agent, Read" {
				t.Errorf("allowed-tools = %v: the CLI, the architect and reading only", s.frontmatter["allowed-tools"])
			}
			if !strings.Contains(s.body, "`athel:architect`") || !strings.Contains(s.body, "--result blocked") {
				t.Error("step skills delegate to the architect and end a refused step as blocked")
			}
			at := 0
			for _, want := range order {
				i := strings.Index(s.body[at:], want)
				if i < 0 {
					t.Fatalf("%q missing or out of order", want)
				}
				at += i
			}
		})
	}
}

// TestHumanSkillsAreHumanOnly: the model must not invoke the approval skills itself (the grant
// would not exist anyway, because only the human's own prompt creates one).
func TestHumanSkillsAreHumanOnly(t *testing.T) {
	for name, call := range map[string]string{
		"approve": "--to plan_approved --by human",
		"accept":  "--to done --by human",
		"decide":  "decide answer <CHG-NNN> <answers>",
		"signoff": "blast signoff $ARGUMENTS",
	} {
		s := readSkill(t, name)
		if s.frontmatter["disable-model-invocation"] != true {
			t.Errorf("%s: disable-model-invocation must be true", name)
		}
		if !strings.Contains(s.body, call) || !strings.Contains(s.body, "E-POLICY-HUMAN-ORIGIN") {
			t.Errorf("%s: body must run %q and explain its denial", name, call)
		}
	}
}

// TestDecideTicksAfterAnswering (ADR 019 follow-up): a due default must never override the
// answers the developer just typed, so the decide skill ticks only after recording them.
func TestDecideTicksAfterAnswering(t *testing.T) {
	body := readSkill(t, "decide").body
	answer, tick := strings.Index(body, "decide answer"), strings.Index(body, "decide tick")
	if answer < 0 || tick < answer {
		t.Errorf("decide answer at %d, decide tick at %d: tick only after the answers", answer, tick)
	}
}
