package grants

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
)

// TargetDecide is the grant target for answering decision cards (doc 05 FR-DEC-004; ADR 019).
// Only the developer's own `/athel:decide CHG-NNN D-1=b …` prompt can allow `athel decide
// answer` with exactly those answers: the model can neither answer its own cards nor change an
// answer on the way to the CLI.
const TargetDecide = "decision_answers"

// Answer is one answer in a decide prompt: an option key, free text, or "should not have asked".
type Answer struct {
	Card   string `json:"card"`
	Option string `json:"option,omitempty"`
	Text   string `json:"text,omitempty"`
	Skip   bool   `json:"skip,omitempty"`
}

var (
	decidePromptRe = regexp.MustCompile(`^\s*/athel:decide\s+(CHG-[0-9]{3,})((?:\s+\S.*)?)\s*$`)
	// tokenRe matches D-1=b, D-2='free text' and skip=D-3. Free text is single-quoted and holds no
	// quote, so the same words pass the shell unchanged into the CLI call the hook names.
	tokenRe = regexp.MustCompile(`^(?:(D-[0-9]+)=(?:([a-d])|'([^'"\x60$\\]{1,500})')|skip=(D-[0-9]+))$`)
)

// ParseDecidePrompt returns the change and answers of a literal `/athel:decide` prompt. Every
// word after the change must be an answer; anything else makes it not a decide prompt.
func ParseDecidePrompt(prompt string) (string, []Answer, bool) {
	m := decidePromptRe.FindStringSubmatch(prompt)
	if m == nil {
		return "", nil, false
	}
	answers, ok := ParseAnswers(splitWords(m[2]))
	if !ok || len(answers) == 0 {
		return "", nil, false
	}
	return m[1], answers, true
}

// splitWords splits on spaces outside single quotes, keeping the quotes.
func splitWords(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '\'':
			quoted = !quoted
			cur.WriteRune(r)
		case !quoted && (r == ' ' || r == '\t'):
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// ParseAnswers reads answer words (the prompt's, or the CLI's arguments with shell quoting
// already removed and re-added by AnswerArgs), sorted by card so the same answers always give
// the same grant key.
func ParseAnswers(words []string) ([]Answer, bool) {
	var out []Answer
	for _, w := range words {
		m := tokenRe.FindStringSubmatch(w)
		if m == nil {
			return nil, false
		}
		switch {
		case m[4] != "":
			out = append(out, Answer{Card: m[4], Skip: true})
		case m[2] != "":
			out = append(out, Answer{Card: m[1], Option: m[2]})
		default:
			out = append(out, Answer{Card: m[1], Text: strings.TrimSpace(m[3])})
		}
	}
	slices.SortStableFunc(out, func(a, b Answer) int { return strings.Compare(a.word(), b.word()) })
	return out, true
}

// word is an answer as typed: D-1=b, D-2='text' or skip=D-3.
func (a Answer) word() string {
	switch {
	case a.Skip:
		return "skip=" + a.Card
	case a.Option != "":
		return a.Card + "=" + a.Option
	default:
		return a.Card + "='" + a.Text + "'"
	}
}

// DecideKey binds a grant to one change and exactly these answers.
func DecideKey(change string, answers []Answer) string {
	words := make([]string, 0, len(answers))
	for _, a := range answers {
		words = append(words, a.word())
	}
	return "decide:" + digest.Bytes([]byte(change + "\n" + strings.Join(words, "\n")))[7:23]
}

// AnswerArgs are the CLI arguments for answers: each answer one shell word, single-quoted.
func AnswerArgs(answers []Answer) []string {
	out := make([]string, 0, len(answers))
	for _, a := range answers {
		out = append(out, "'"+strings.ReplaceAll(a.word(), "'", `'"'"'`)+"'")
	}
	return out
}

// DecideCommand is the exact command that records the answers.
func DecideCommand(change string, answers []Answer) string {
	return "athel decide answer " + change + " " + strings.Join(AnswerArgs(answers), " ")
}
