package grants

import (
	"regexp"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
)

// TargetWaive is the grant target for waiving a gate or a review finding (doc 06 FR-BUILD-014:
// "gate waivers require a human, a reason and an expiry"). Only the developer's own
// `/athel:waive` prompt can allow `athel gate waive` with exactly its task, subject, expiry and
// reason.
const TargetWaive = "gate_waiver"

// Waiver is what a waive prompt names.
type Waiver struct {
	Change, Task, Subject, Until, Reason string
}

// The reason is single-quoted and holds no quote, backquote, dollar or backslash, so it passes
// the shell unchanged into the command the hook names (as decide's free text).
var waivePromptRe = regexp.MustCompile(`^\s*/athel:waive\s+(CHG-[0-9]{3,})\s+(T-[0-9]{2,})\s+([a-z][a-z0-9-]{1,40}|R[1-9][0-9]*-[0-9]{2,})\s+until=([0-9]{4}-[0-9]{2}-[0-9]{2})\s+'([^'"\x60$\\]{1,500})'\s*$`)

// ParseWaivePrompt returns the waiver of a literal `/athel:waive` prompt.
func ParseWaivePrompt(prompt string) (Waiver, bool) {
	m := waivePromptRe.FindStringSubmatch(prompt)
	if m == nil || strings.TrimSpace(m[5]) == "" {
		return Waiver{}, false
	}
	return Waiver{Change: m[1], Task: m[2], Subject: m[3], Until: m[4], Reason: m[5]}, true
}

// WaiveKey binds a grant to one waiver, every field of it.
func WaiveKey(w Waiver) string {
	return "waive:" + digest.Bytes([]byte(strings.Join([]string{w.Change, w.Task, w.Subject, w.Until, w.Reason}, "\n")))[7:23]
}

// WaiveCommand is the exact command that records the waiver.
func WaiveCommand(w Waiver) string {
	return "athel gate waive " + w.Change + " " + w.Task + " " + w.Subject + " --until " + w.Until + " --reason '" + w.Reason + "'"
}
