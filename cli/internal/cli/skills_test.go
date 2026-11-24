package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func skill(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "plugin", "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSkillsNameRealCommands: every athel command a skill tells the model to run exists, so a
// skill never sends it guessing.
func TestSkillsNameRealCommands(t *testing.T) {
	cmds := commands()
	call := regexp.MustCompile("`athel (?:--json )?(?:--session \\$\\{CLAUDE_SESSION_ID\\} )?([a-z]+)(?: ([a-z]+))?(?: ([a-z]+))?")
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "plugin", "skills", "*", "SKILL.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no skills: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range call.FindAllStringSubmatch(string(b), -1) {
			_, three := cmds[strings.TrimSpace(m[1]+" "+m[2]+" "+m[3])]
			_, two := cmds[strings.TrimSpace(m[1]+" "+m[2])]
			_, one := cmds[m[1]]
			if !three && !two && !one {
				t.Errorf("%s names athel %s %s %s, which does not exist", f, m[1], m[2], m[3])
			}
		}
	}
}

// inOrder reports the first step of want that does not follow the one before it in text.
func inOrder(text string, want []string) string {
	at := 0
	for _, w := range want {
		i := strings.Index(text[at:], w)
		if i < 0 {
			return w
		}
		at += i + len(w)
	}
	return ""
}

// TestBuildSkillFollowsTheLoop keeps /athel:build in doc 06's per-task order (M4-plan §4.1):
// the developer from its manifest, cheap gates, the three checkers from theirs, QA's tests
// committed by the skill, full gates, done, then push and PR.
// TestDelegatingSkillsWaitInTheForeground: a headless step whose agents run in the background
// ends its turn before they report, and the step stops half done (M4 full eval).
func TestDelegatingSkillsWaitInTheForeground(t *testing.T) {
	for _, name := range []string{"spec", "plan", "investigate", "blast-radius", "ask"} {
		if text := skill(t, name); !strings.Contains(text, foregroundRule) {
			t.Errorf("/athel:%s delegates to agents but does not keep them in the foreground", name)
		}
	}
}

func TestBuildSkillFollowsTheLoop(t *testing.T) {
	text := skill(t, "build")
	loop := []string{"git preflight", "--to building", "build next", "state enter $ARGUMENTS build --task", "gate baseline",
		"build context", "`athel:developer`", "gate run --stage cheap", "review context", "`athel:qa`", "`athel:security`",
		"`athel:reviewer`", "commit -m", "gate run --stage full", "build done", "git push", "pr open", "pr update",
		"state exit $ARGUMENTS build --result ok", "--result blocked"}
	if step := inOrder(text, loop); step != "" {
		t.Errorf("/athel:build is out of order at %q", step)
	}
	for _, never := range []string{"gate waive", "decide answer", "finding triage"} {
		if strings.Contains(text, "--session ${CLAUDE_SESSION_ID} "+never) {
			t.Errorf("/athel:build runs %q", never)
		}
	}
	if !strings.Contains(text, "in the foreground") || !strings.Contains(text, "never start one in\n  the background") {
		t.Error("/athel:build must keep its agents in the foreground: a turn that ends to wait counts as a stop")
	}
	if strings.Contains(text, "`git push origin") || strings.Contains(text, "`gh pr ") {
		t.Error("/athel:build pushes or edits a PR without the CLI (ADR 026)")
	}
}

// TestShipSkillHandsOverWithoutMerging: /athel:ship checks the real blast radius, presents the
// side findings for the developer's choice, and marks the PRs ready; it never merges.
func TestShipSkillHandsOverWithoutMerging(t *testing.T) {
	text := skill(t, "ship")
	steps := []string{"--to ready_for_pr", "state enter $ARGUMENTS ship", "blast check", "finding list", "/athel:triage",
		"finding fix", "pr update", "pr ready", "--to in_review", "state exit $ARGUMENTS ship --result ok"}
	if step := inOrder(text, steps); step != "" {
		t.Errorf("/athel:ship is out of order at %q", step)
	}
	if strings.Contains(text, "pr merge") || strings.Contains(text, "finding triage") {
		t.Error("/athel:ship merges, or triages for the developer")
	}
}

const foregroundRule = "Run every subagent in the foreground and wait for its result in the same turn"

// TestSkillsHandleChangeSets (M5-plan task 13): the skills run a multi-repo change as one set:
// plan shows the merge order, build goes on past a held task, ship rehearses the merge order
// before any PR is marked ready, status shows the whole set.
func TestSkillsHandleChangeSets(t *testing.T) {
	ship := skill(t, "ship")
	if v, r := strings.Index(ship, "set verify"), strings.Index(ship, "pr ready"); v < 0 || r < 0 || v > r {
		t.Error("ship must rehearse (set verify) before it marks PRs ready (pr ready)")
	}
	for name, wants := range map[string][]string{
		"plan":     {"merge_order", "coordinated-release card"},
		"build":    {"`next`", "build park", "blocked"},
		"status":   {"set status", "alarms", "source"},
		"followup": {"followup draft", "disable-model-invocation: true"},
	} {
		text := skill(t, name)
		for _, w := range wants {
			if !strings.Contains(text, w) {
				t.Errorf("%s skill lacks %q", name, w)
			}
		}
	}
}
