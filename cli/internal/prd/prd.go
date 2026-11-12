// Package prd gives a PRD's headings and requirement-like sentences stable IDs (doc 05
// FR-SPEC-003), diffs PRD versions, and pre-scans for vague words and numeric limits for the
// architect's review (FR-SPEC-002). Everything here is deterministic: the same text always gets
// the same IDs, and a new version keeps the IDs of unchanged text.
package prd

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Item is one indexed heading or statement.
type Item struct {
	ID   string `yaml:"id" json:"id"`
	Kind string `yaml:"kind" json:"kind"`
	Line int    `yaml:"line" json:"line"`
	Text string `yaml:"text" json:"text"`
}

// Index is prd/<version>.index.yaml (athel/prd-index@1).
type Index struct {
	Schema       string `yaml:"schema" json:"schema"`
	Change       string `yaml:"change" json:"change"`
	Version      string `yaml:"version" json:"version"`
	SourceSHA256 string `yaml:"source_sha256" json:"source_sha256"`
	Items        []Item `yaml:"items" json:"items"`
}

// SchemaID is the index's schema.
const SchemaID = "athel/prd-index@1"

var (
	headingRe = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	listRe    = regexp.MustCompile(`^\s*(?:[-*+]|[0-9]+[.)])\s+(.+)$`)
	modalRe   = regexp.MustCompile(`(?i)\b(shall|must|should|will|can|cannot|may not|needs? to|has to|have to)\b`)
	// A sentence ends at . ! or ? followed by a space or the line's end, so a dot inside a token
	// (E.164, v1.2, 2.5) stays in its sentence.
	sentRe  = regexp.MustCompile(`(?:[^.!?]|[.!?]+[^.!?\s])+(?:[.!?]+|$)`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// candidate is a heading or statement before it has an ID.
type candidate struct {
	kind, text string
	line       int
}

// parse reads the PRD's headings, list items and modal sentences in order. Code blocks are
// skipped: they hold examples, not requirements.
func parse(text []byte) []candidate {
	var out []candidate
	inCode := false
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			continue
		}
		if inCode || strings.TrimSpace(line) == "" {
			continue
		}
		if m := headingRe.FindStringSubmatch(line); m != nil {
			out = append(out, candidate{"heading", normalise(m[1]), n})
			continue
		}
		if m := listRe.FindStringSubmatch(line); m != nil {
			out = append(out, candidate{"statement", normalise(m[1]), n})
			continue
		}
		for _, s := range sentRe.FindAllString(line, -1) {
			if s = normalise(s); s != "" && modalRe.MatchString(s) {
				out = append(out, candidate{"statement", s, n})
			}
		}
	}
	return out
}

func normalise(s string) string { return strings.TrimSpace(spaceRe.ReplaceAllString(s, " ")) }

// Build indexes text. With prev (the previous version's index), unchanged headings and
// statements keep their IDs; new ones get the next free number. Statements before the first
// heading belong to heading 0.
func Build(change, version, sha string, text []byte, prev *Index) Index {
	reuse := map[string]string{} // kind + text → ID
	nextHeading := 1
	nextStmt := map[int]int{}
	if prev != nil {
		for _, it := range prev.Items {
			reuse[it.Kind+"\x00"+it.Text] = it.ID
			h, k := split(it.ID)
			nextHeading = max(nextHeading, h+1)
			nextStmt[h] = max(nextStmt[h], k+1)
		}
	}
	idx := Index{Schema: SchemaID, Change: change, Version: version, SourceSHA256: sha, Items: []Item{}}
	used := map[string]bool{}
	heading := 0
	for _, c := range parse(text) {
		id, ok := reuse[c.kind+"\x00"+c.text]
		if ok && !used[id] {
			if c.kind == "heading" {
				heading, _ = split(id)
			}
		} else if c.kind == "heading" {
			heading, nextHeading = nextHeading, nextHeading+1
			id = "PRD-" + strconv.Itoa(heading)
		} else {
			k := max(nextStmt[heading], 1)
			nextStmt[heading] = k + 1
			id = fmt.Sprintf("PRD-%d.%d", heading, k)
		}
		used[id] = true
		idx.Items = append(idx.Items, Item{ID: id, Kind: c.kind, Line: c.line, Text: c.text})
	}
	return idx
}

// split reads PRD-<h> or PRD-<h>.<k>; IDs come from this package or a validated index.
func split(id string) (int, int) {
	h, k, _ := strings.Cut(strings.TrimPrefix(id, "PRD-"), ".")
	hi, _ := strconv.Atoi(h)
	ki, _ := strconv.Atoi(k)
	return hi, ki
}

// Has reports whether the index holds id.
func (x Index) Has(id string) bool {
	for _, it := range x.Items {
		if it.ID == id {
			return true
		}
	}
	return false
}

// Diff is what changed between two versions, by ID.
type Diff struct {
	Added   []Item `json:"added"`
	Removed []Item `json:"removed"`
}

// Compare lists the IDs only in b (added) and only in a (removed). A reworded statement shows as
// one removed and one added, so specs citing the old ID are found by the trace check.
func Compare(a, b Index) Diff {
	in := func(x Index) map[string]Item {
		m := map[string]Item{}
		for _, it := range x.Items {
			m[it.ID] = it
		}
		return m
	}
	ma, mb := in(a), in(b)
	d := Diff{Added: []Item{}, Removed: []Item{}}
	for _, it := range b.Items {
		if _, ok := ma[it.ID]; !ok {
			d.Added = append(d.Added, it)
		}
	}
	for _, it := range a.Items {
		if _, ok := mb[it.ID]; !ok {
			d.Removed = append(d.Removed, it)
		}
	}
	return d
}

// Finding is a pre-scan hit: a vague word or a numeric limit in one indexed item.
type Finding struct {
	PRD    string `json:"prd"`
	Phrase string `json:"phrase"`
}

// vague are words that hide a requirement (doc 05 FR-SPEC-002: "fast", "secure", "etc.").
var vague = []string{"fast", "quick", "quickly", "secure", "safe", "easy", "simple", "intuitive", "user-friendly",
	"seamless", "scalable", "robust", "reliable", "flexible", "efficient", "performant", "responsive", "modern",
	"etc.", "and so on", "as needed", "if needed", "as appropriate", "appropriate", "reasonable", "minimal",
	"optimal", "best", "better", "improve", "enhance", "support", "handle", "some", "various", "real-time"}

var limitRe = regexp.MustCompile(`(?i)\b[0-9]+(?:\.[0-9]+)?\s*(?:%|(?:ms|milliseconds?|s|seconds?|minutes?|hours?|days?|percent|rps|requests?|calls?|users?|mb|gb|kb|items?|retries|times)\b)`)

// Scan lists vague words and numeric limits per item, for the architect to judge: which
// ambiguities are material, and whether two limits on the same thing contradict each other.
func Scan(x Index) (ambiguous, limits []Finding) {
	ambiguous, limits = []Finding{}, []Finding{}
	for _, it := range x.Items {
		lower := " " + strings.ToLower(it.Text) + " "
		for _, w := range vague {
			if containsWord(lower, w) {
				ambiguous = append(ambiguous, Finding{PRD: it.ID, Phrase: w})
			}
		}
		for _, m := range limitRe.FindAllString(it.Text, -1) {
			limits = append(limits, Finding{PRD: it.ID, Phrase: m})
		}
	}
	return ambiguous, limits
}

// containsWord matches w at word boundaries in s (which is padded with spaces).
func containsWord(s, w string) bool {
	for i := strings.Index(s, w); i >= 0; {
		before, after := s[i-1], byte(' ')
		if end := i + len(w); end < len(s) {
			after = s[end]
		}
		if !isWordChar(before) && (!isWordChar(after) || strings.HasSuffix(w, ".")) {
			return true
		}
		j := strings.Index(s[i+1:], w)
		if j < 0 {
			return false
		}
		i += j + 1
	}
	return false
}

func isWordChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// Encode writes an index as YAML. An index holds strings and integers only, so it always encodes.
func Encode(x Index) []byte {
	b, _ := yaml.Marshal(x)
	return b
}

// DecodeIndex reads an index strictly and validates it against prd-index@1.
func DecodeIndex(reg *schema.Registry, data []byte) (*Index, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var x Index
	if err := dec.Decode(&x); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode PRD index", err)
	}
	if err := reg.ValidateYAML(schema.PRDIndex, data); err != nil {
		return nil, err
	}
	return &x, nil
}
