// Package observe looks at tool output after the call: it flags text that reads like
// instructions to an AI (doc 04 injection-defence layer 3) and keeps the scout's tool outputs,
// so facts are checked against what the tools returned, not what the model says they returned
// (M2 plan G2).
package observe

import (
	"regexp"
	"strings"
)

// Labels of the suspicious-content patterns.
const (
	LabelIgnorePrevious = "ignore previous instructions"
	LabelAddressedToAI  = "instructions addressed to an AI"
	LabelYouMustNow     = "you must now"
	LabelSystemPrompt   = "injected system prompt"
	LabelRunCommand     = "run the following command"
)

// MaxExcerpt bounds the text kept around a match.
const MaxExcerpt = 200

// patterns are deliberately narrow: a flag adds a warning to Claude's context and to the audit
// log, so ordinary logs must not trip it. The verbs are the actions an injection wants.
var patterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{LabelIgnorePrevious, regexp.MustCompile(`(?i)\b(ignore|disregard|forget|override)\s+(all\s+|any\s+)?(the\s+|your\s+)?(previous|prior|above|earlier|preceding|existing)\s+(instructions|prompts?|messages|rules|guidelines|directions)\b`)},
	{LabelAddressedToAI, regexp.MustCompile(`(?i)(@?\bathel\b|\bclaude\b|\bchatgpt\b|\bai\s+(agent|assistant)\b|\bllm\b|\blanguage\s+model\b)[\s,:-]+(please\s+)?(run|execute|delete|push|deploy|merge|ignore|disregard)\b`)},
	{LabelYouMustNow, regexp.MustCompile(`(?i)\byou\s+(must|should|need\s+to|have\s+to)\s+now\s+(run|execute|delete|push|deploy|merge|ignore|disregard|forget)\b`)},
	{LabelSystemPrompt, regexp.MustCompile(`(?i)\bsystem\s+prompt\s*:|\bnew\s+instructions\s*:`)},
	{LabelRunCommand, regexp.MustCompile(`(?i)\b(run|execute)\s+(the\s+following|this)\s+(command|script)\b`)},
}

// Flag is one suspicious passage: which pattern matched and the text around it.
type Flag struct {
	Phrase  string `json:"phrase"`
	Excerpt string `json:"excerpt"`
}

var space = regexp.MustCompile(`\s+`)

// Suspicious returns one flag per matching pattern, in pattern order.
func Suspicious(text string) []Flag {
	var out []Flag
	for _, p := range patterns {
		loc := p.re.FindStringIndex(text)
		if loc == nil {
			continue
		}
		out = append(out, Flag{Phrase: p.label, Excerpt: excerpt(text, loc[0], loc[1])})
	}
	return out
}

// excerpt is the match with some context on each side, on one line, at most MaxExcerpt bytes.
func excerpt(text string, start, end int) string {
	from, to := max(0, start-60), min(len(text), end+60)
	ex := strings.TrimSpace(space.ReplaceAllString(text[from:to], " "))
	if len(ex) > MaxExcerpt {
		ex = strings.ToValidUTF8(ex[:MaxExcerpt], "")
	}
	return ex
}

// Strings lists the string leaves of a decoded JSON value, where tool output text lives.
func Strings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, Strings(e)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, e := range t {
			out = append(out, Strings(e)...)
		}
		return out
	}
	return nil
}
