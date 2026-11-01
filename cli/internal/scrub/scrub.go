// Package scrub removes secret values from text before it reaches the model or the audit log
// (doc 04 §Secrets, FR-SAFE-008). Matches become [REDACTED:<kind>]. It is deliberately greedy:
// a redacted non-secret costs a little context; a leaked secret cannot be taken back.
package scrub

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// rule is one detector. If group > 0, only that capture group is replaced (the label stays
// readable, e.g. "password=" in a connection string).
type rule struct {
	kind  string
	re    *regexp.Regexp
	group int
	check func(string) bool // optional extra test on the matched secret
}

// minEntropyLen and minEntropy define "high-entropy string over 32 characters" (doc 04).
const (
	minEntropyLen = 32
	minEntropy    = 3.5 // bits per character
)

var rules = []rule{
	{kind: "private-key", re: regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?(-----END [A-Z0-9 ]*PRIVATE KEY-----|\z)`)},
	{kind: "aws-access-key-id", re: regexp.MustCompile(`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA)[A-Z0-9]{16}\b`)},
	{kind: "github-token", re: regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{22,})\b`)},
	{kind: "slack-token", re: regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`)},
	{kind: "google-api-key", re: regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`)},
	{kind: "anthropic-key", re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}`)},
	{kind: "openai-key", re: regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_\-]{32,}`)},
	{kind: "stripe-key", re: regexp.MustCompile(`\b[rs]k_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{kind: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`)},
	{kind: "connection-string-password", re: regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]*://[^\s:/@"']+:([^\s@/"']+)@`), group: 1},
	{kind: "secret-env", re: regexp.MustCompile(`\b[A-Z][A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_KEY|APIKEY|PRIVATE_KEY|ACCESS_KEY|CREDENTIALS?)[A-Z0-9_]*\s*[=:]\s*["']?([^\s"']+)`), group: 1},
	{kind: "password", re: regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|passphrase)\b["']?\s*[:=]\s*["']?([^\s"',;}]{4,})`), group: 1},
	{kind: "high-entropy-secret", re: regexp.MustCompile(`(?i)\b(?:[a-z0-9_\-]*(?:key|token|secret|credential|auth))\b["']?\s*[:=]\s*["']?([A-Za-z0-9+/_\-.=]{32,})`), group: 1, check: highEntropy},
}

// Finding counts redactions of one kind.
type Finding struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// Scrubber redacts secrets. Extra literal values (for example secrets named in config whose
// values the CLI resolved itself) are redacted wherever they appear.
type Scrubber struct {
	literals []string
}

// New returns a scrubber that also redacts the given literal values (empty and short values,
// which would redact ordinary words, are ignored).
func New(literals ...string) *Scrubber {
	s := &Scrubber{}
	for _, l := range literals {
		if len(l) >= 8 {
			s.literals = append(s.literals, l)
		}
	}
	sort.Slice(s.literals, func(i, j int) bool { return len(s.literals[i]) > len(s.literals[j]) })
	return s
}

// String returns text with every detected secret replaced, and what was found.
func (s *Scrubber) String(text string) (string, []Finding) {
	counts := map[string]int{}
	for _, l := range s.literals {
		if n := strings.Count(text, l); n > 0 {
			text = strings.ReplaceAll(text, l, "[REDACTED:configured-secret]")
			counts["configured-secret"] += n
		}
	}
	for _, r := range rules {
		text = apply(r, text, counts)
	}
	return text, findings(counts)
}

func apply(r rule, text string, counts map[string]int) string {
	return r.re.ReplaceAllStringFunc(text, func(m string) string {
		sub := r.re.FindStringSubmatchIndex(m)
		if r.group == 0 {
			if r.check != nil && !r.check(m) {
				return m
			}
			counts[r.kind]++
			return "[REDACTED:" + r.kind + "]"
		}
		start, end := sub[2*r.group], sub[2*r.group+1]
		secret := m[start:end]
		if strings.HasPrefix(secret, "[REDACTED:") || (r.check != nil && !r.check(secret)) {
			return m
		}
		counts[r.kind]++
		return m[:start] + "[REDACTED:" + r.kind + "]" + m[end:]
	})
}

func findings(counts map[string]int) []Finding {
	out := make([]Finding, 0, len(counts))
	for k, n := range counts {
		out = append(out, Finding{Kind: k, Count: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

// highEntropy reports whether s is long and random enough to be a credential rather than a
// word, path or identifier.
func highEntropy(s string) bool {
	if len(s) < minEntropyLen {
		return false
	}
	return Entropy(s) >= minEntropy
}

// Entropy is the Shannon entropy of s in bits per character.
func Entropy(s string) float64 {
	if s == "" {
		return 0
	}
	freq := map[rune]float64{}
	n := 0.0
	for _, r := range s {
		freq[r]++
		n++
	}
	h := 0.0
	for _, f := range freq {
		p := f / n
		h -= p * math.Log2(p)
	}
	return h
}

// Value scrubs every string inside a decoded JSON value, keeping its shape, so a tool's output
// can be replaced with a redacted copy of the same structure (Claude Code requires the shape of
// updatedToolOutput to match the tool's output).
func (s *Scrubber) Value(v any) (any, []Finding) {
	counts := map[string]int{}
	out := s.walk(v, counts)
	return out, findings(counts)
}

func (s *Scrubber) walk(v any, counts map[string]int) any {
	switch t := v.(type) {
	case string:
		clean, fs := s.String(t)
		for _, f := range fs {
			counts[f.Kind] += f.Count
		}
		return clean
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = s.walk(e, counts)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = s.walk(e, counts)
		}
		return out
	}
	return v
}
