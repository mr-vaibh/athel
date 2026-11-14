package quality

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestRuleTablesMatchCLI keeps the reviewer's rule table equal to the CLI's list (M4-plan
// §4.5 prompt-sync): an ID the prompt offers is one the CLI accepts, and the other way round.
func TestRuleTablesMatchCLI(t *testing.T) {
	b, err := os.ReadFile("../../../plugin/agents/reviewer.md")
	if err != nil {
		t.Fatal(err)
	}
	_, table, ok := strings.Cut(string(b), "## Rule IDs")
	if !ok {
		t.Fatal("reviewer.md has no Rule IDs section")
	}
	table, _, _ = strings.Cut(table, "\n## ")
	var prompt []string
	for _, m := range regexp.MustCompile("(?m)^\\| `([^`]+)` \\| (.+) \\|$").FindAllStringSubmatch(table, -1) {
		prompt = append(prompt, m[1]+" = "+m[2])
	}
	var cli []string
	for _, r := range Rules {
		cli = append(cli, r.ID+" = "+r.Text)
	}
	if !slices.Equal(prompt, cli) {
		t.Errorf("reviewer.md's rule table differs from quality.Rules:\nprompt %v\ncli    %v", prompt, cli)
	}
}

func TestRuleIDs(t *testing.T) {
	form := regexp.MustCompile(`^[a-z]+\.[a-z-]+$`)
	seen := map[string]bool{}
	for _, r := range Rules {
		if !form.MatchString(r.ID) || seen[r.ID] || r.Text == "" {
			t.Errorf("rule %+v: an ID is area.name, once, with its text", r)
		}
		seen[r.ID] = true
	}
	if !Known("errors.no-silent-catch") || Known("style.tabs") {
		t.Error("Known")
	}
}
