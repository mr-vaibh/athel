package grants

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
)

// TargetTriage is the grant target for the developer's side-finding dispositions (doc 07
// FR-FIND-003: "the human chooses: separate fix PR, ticket, or ignore"). Only the developer's own
// `/athel:triage` prompt allows `athel finding triage` with exactly its choices.
const TargetTriage = "finding_triage"

var (
	triagePromptRe = regexp.MustCompile(`^\s*/athel:triage\s+(CHG-[0-9]{3,})((?:\s+\S+)+)\s*$`)
	choiceRe       = regexp.MustCompile(`^(SF-[1-9][0-9]*|all)=(fix_pr|ticket|ignore)$`)
)

// ParseTriagePrompt returns the change and choices of a literal `/athel:triage` prompt, sorted:
// one choice per finding (SF-1=fix_pr …) or one for all (all=ticket), never both.
func ParseTriagePrompt(prompt string) (string, []string, bool) {
	m := triagePromptRe.FindStringSubmatch(prompt)
	if m == nil {
		return "", nil, false
	}
	choices, ok := ParseChoices(strings.Fields(m[2]))
	return m[1], choices, ok
}

// ParseChoices checks triage words: each SF-n or all, once, with a disposition; sorted.
func ParseChoices(words []string) ([]string, bool) {
	seen := map[string]bool{}
	for _, w := range words {
		c := choiceRe.FindStringSubmatch(w)
		if c == nil || seen[c[1]] {
			return nil, false
		}
		seen[c[1]] = true
	}
	if len(words) == 0 || (seen["all"] && len(words) > 1) {
		return nil, false
	}
	out := slices.Clone(words)
	slices.Sort(out)
	return out, true
}

// TriageKey binds a grant to one change and exactly these choices.
func TriageKey(change string, choices []string) string {
	return "triage:" + digest.Bytes([]byte(change + "\n" + strings.Join(choices, "\n")))[7:23]
}

// TriageCommand is the exact command that records the choices.
func TriageCommand(change string, choices []string) string {
	return "athel finding triage " + change + " " + strings.Join(choices, " ")
}
