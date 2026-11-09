package hooks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// maxSummaries bounds the SessionStart context; the rest are counted, not listed.
const maxSummaries = 10

func sessionStart(req *hookio.Request, d Deps) Outcome {
	o := Outcome{Rule: RuleLogOnly, Result: "ok"}
	var lines []string
	if d.EnvFile != "" && d.AppendEnv != nil {
		if err := d.AppendEnv(d.EnvFile, "export ATHEL_SESSION_ID="+req.SessionID); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "export session: "+err.Error())
		}
	}
	lines = append(lines, versionLine(d)...)
	if d.ConfigErr != nil {
		o.Result = "error"
		lines = append(lines, "Athel: CONFIGURATION INVALID, fix it before any work: "+d.ConfigErr.Error())
	}
	if d.Store == nil {
		lines = append(lines, "Athel: no .athel/ workspace here; run `athel init` to start.")
		o.Response.AdditionalContext = strings.Join(lines, "\n")
		return o
	}
	changes, err := d.Store.List()
	if err != nil {
		o.Result = "error"
		lines = append(lines, "Athel: could not read changes: "+err.Error())
		o.Response.AdditionalContext = strings.Join(lines, "\n")
		return o
	}
	lines = append(lines, summaryLines(changes, req.SessionID)...)
	o.Response.AdditionalContext = strings.Join(lines, "\n")
	housekeeping(d, &o)
	return o
}

// housekeeping drops expired observations and archives idle facts (doc 03 §Size and cleanup).
// It never holds up the session: a failure is recorded in the audit line and nothing else.
func housekeeping(d Deps, o *Outcome) {
	if d.Observe != nil {
		if _, err := d.Observe.Prune(); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "prune observations: "+err.Error())
		}
	}
	if d.Facts != nil {
		if _, err := d.Facts.Archive(false); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "archive facts: "+err.Error())
		}
	}
	if d.RefreshIndex != nil {
		if err := d.RefreshIndex(); err != nil {
			o.Result, o.Notes = "error", append(o.Notes, "refresh repo index: "+err.Error())
		}
	}
}

func summaryLines(changes []*state.Change, session string) []string {
	var active []*state.Change
	for _, c := range changes {
		if !state.IsTerminal(c.State) {
			active = append(active, c)
		}
	}
	if len(active) == 0 {
		return []string{"Athel: no active changes. Start one with /athel:start."}
	}
	lines := []string{fmt.Sprintf("Athel: %d active change(s). Run /athel:resume to continue one. Never edit .athel/ files by hand; use the athel CLI.", len(active))}
	for i, c := range active {
		if i == maxSummaries {
			lines = append(lines, fmt.Sprintf("- … and %d more (athel change list)", len(active)-maxSummaries))
			break
		}
		lines = append(lines, "- "+summaryLine(c, session))
	}
	return lines
}

func summaryLine(c *state.Change, session string) string {
	parts := []string{fmt.Sprintf("%s %q: %s", c.ID, c.Title, c.State)}
	if c.Step != nil {
		parts = append(parts, "step "+c.Step.Name+" "+c.Step.Status)
	}
	if cp := state.LastCheckpoint(c); cp != nil {
		parts = append(parts, "last checkpoint "+cp.Name)
	}
	if c.Budget.Status != state.BudgetOK {
		parts = append(parts, "budget "+c.Budget.Status)
	}
	if c.Lock != nil && c.Lock.Session != session {
		parts = append(parts, "locked by another session ("+c.Lock.Session+")")
	}
	if c.Tampered() {
		parts = append(parts, "EDITED OUTSIDE ATHEL, frozen until the developer runs `athel integrity accept "+c.ID+"`")
	}
	return strings.Join(parts, ", ")
}

// versionLine warns when the running Claude Code is older than the minimum tested version. The
// version is not in the hook input, so it comes from `claude --version` (M0-plan §6.4).
func versionLine(d Deps) []string {
	if d.ClaudeVersion == nil || d.MinClaudeCode == "" {
		return nil
	}
	running, err := d.ClaudeVersion()
	if err != nil {
		return []string{"Athel: Claude Code version unknown (" + err.Error() + "); tested with " + d.MinClaudeCode + " or later."}
	}
	if Older(running, d.MinClaudeCode) {
		return []string{fmt.Sprintf("Athel: WARNING Claude Code %s is older than the minimum tested version %s.", running, d.MinClaudeCode)}
	}
	return nil
}

// Older compares dotted numeric versions ("2.1.308"); text after the first space is ignored.
// An unparseable version is treated as older, so the warning errs on the side of showing.
func Older(running, minimum string) bool {
	r, okR := parseVersion(running)
	m, okM := parseVersion(minimum)
	if !okR || !okM {
		return true
	}
	for i := range max(len(r), len(m)) {
		a, b := at(r, i), at(m, i)
		if a != b {
			return a < b
		}
	}
	return false
}

func parseVersion(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, ' '); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func at(v []int, i int) int {
	if i < len(v) {
		return v[i]
	}
	return 0
}
