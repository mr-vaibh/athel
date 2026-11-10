// Package ids allocates and parses change, fact and question IDs. IDs come from the CLI, never from the model.
package ids

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// changeRe matches CHG-NNN and child IDs such as CHG-042-F3 (side-finding fix changes, doc 07).
var changeRe = regexp.MustCompile(`^CHG-([0-9]{3,})(?:-F([1-9][0-9]*))?$`)

// ChangeID is a validated change identifier.
type ChangeID struct {
	Number int
	Child  int // 0 for a top-level change
}

// FormatChange renders a top-level change ID: at least three digits, zero padded.
func FormatChange(n int) string { return fmt.Sprintf("CHG-%03d", n) }

func (c ChangeID) String() string {
	if c.Child == 0 {
		return FormatChange(c.Number)
	}
	return fmt.Sprintf("%s-F%d", FormatChange(c.Number), c.Child)
}

// ParseChange validates s. Only the canonical spelling is accepted, so one change can never
// have two IDs (CHG-42 and CHG-0042 are rejected; CHG-042 is the only form).
func ParseChange(s string) (ChangeID, error) {
	m := changeRe.FindStringSubmatch(s)
	if m == nil {
		return ChangeID{}, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a change ID (want CHG-NNN)", s))
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n == 0 || FormatChange(n) != "CHG-"+m[1] {
		return ChangeID{}, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a canonical change ID", s))
	}
	id := ChangeID{Number: n}
	if m[2] != "" {
		id.Child, err = strconv.Atoi(m[2])
		if err != nil {
			return ChangeID{}, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q has an invalid child number", s))
		}
	}
	return id, nil
}

// NextChange returns the ID after the highest top-level change in existing. Names that are
// not change IDs (and child IDs) are ignored, so stray folders never break allocation.
func NextChange(existing []string) string {
	highest := 0
	for _, name := range existing {
		id, err := ParseChange(name)
		if err != nil || id.Child != 0 {
			continue
		}
		highest = max(highest, id.Number)
	}
	return FormatChange(highest + 1)
}

// Seq is a family of sequential IDs: a prefix and a minimum digit count, zero padded. Like change
// IDs, only the canonical spelling parses, so one record can never have two IDs.
type Seq struct {
	Prefix string
	Width  int
}

// ID families from M2: facts (doc 03, F-0193) and ad-hoc questions (M2 plan G3).
var (
	Fact = Seq{Prefix: "F-", Width: 4}
	Ask  = Seq{Prefix: "ASK-", Width: 3}
)

// Format renders number n of the family.
func (q Seq) Format(n int) string { return fmt.Sprintf("%s%0*d", q.Prefix, q.Width, n) }

// Parse returns the number of a canonical ID of the family.
func (q Seq) Parse(s string) (int, error) {
	digits, ok := strings.CutPrefix(s, q.Prefix)
	n, err := strconv.Atoi(digits)
	if !ok || err != nil || n <= 0 || q.Format(n) != s {
		return 0, aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a canonical %sID", s, q.Prefix))
	}
	return n, nil
}

// Next returns the ID after the highest one in existing; anything else is ignored.
func (q Seq) Next(existing []string) string {
	highest := 0
	for _, s := range existing {
		if n, err := q.Parse(s); err == nil {
			highest = max(highest, n)
		}
	}
	return q.Format(highest + 1)
}
