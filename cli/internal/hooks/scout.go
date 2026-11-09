package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/hookio"
)

// ScoutUnsaved is why SubagentStop sends the scout back once (doc 02: SubagentStop validates the
// subagent's structured result; an invalid result is re-asked once, then the step fails).
const ScoutUnsaved = "Athel: the scout must end by saving its answer with `athel investigation save` " +
	"(--change or --ask). If the budget ran out, save a partial answer (complete: false) that lists what is missing under gaps."

var agentIDRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

// scoutMarker is the file that records that one scout run (agent_id) saved its answer.
func scoutMarker(d Deps, agentID string) (string, bool) {
	if d.Store == nil || !agentIDRe.MatchString(agentID) {
		return "", false
	}
	return filepath.Join(d.Store.WS.StateDir(), "scout", agentID+".saved"), true
}

// noteScoutSave runs on PostToolUse: a successful `athel investigation save` by the scout marks
// its run as answered. Success is read from the CLI's own JSON output.
func noteScoutSave(req *hookio.Request, d Deps, o *Outcome) {
	marker, ok := scoutMarker(d, req.AgentID)
	if req.AgentType != hookio.ScoutAgent || req.ToolName != "Bash" || !ok || !savesInvestigation(req.ToolInput) {
		return
	}
	var resp struct {
		Stdout string `json:"stdout"`
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(req.ToolResponse, &resp) != nil || json.Unmarshal([]byte(strings.TrimSpace(resp.Stdout)), &out) != nil || !out.OK {
		return
	}
	err := fsutil.EnsureDir(filepath.Dir(marker))
	if err == nil {
		err = fsutil.WriteFileAtomic(marker, []byte(clock.Format(d.Clock.Now())+"\n"))
	}
	if err != nil {
		o.Result, o.Notes = "error", append(o.Notes, "scout save marker: "+err.Error())
	}
}

func savesInvestigation(input []byte) bool {
	res := classify.Classify(classify.Call{Tool: "Bash", Input: input}, nil, classify.Options{})
	for _, a := range res.Actions {
		if a.Athel != nil && a.Athel.Command == "investigation save" {
			return true
		}
	}
	return false
}

// scoutStop lets the scout stop once it has saved its answer. Without one it is sent back once;
// a second stop (stop_hook_active) is let through and recorded, so it can never loop.
func scoutStop(req *hookio.Request, d Deps, o *Outcome) {
	marker, ok := scoutMarker(d, req.AgentID)
	if !ok {
		o.Notes = append(o.Notes, "scout stop without a usable agent_id: not checked")
		return
	}
	if _, err := os.Stat(marker); err == nil {
		_ = os.Remove(marker) // the run is answered; a leftover marker only costs a stale file
		return
	}
	if req.StopHookActive {
		o.Result, o.Notes = "error", append(o.Notes, "the scout ended without saving an investigation")
		return
	}
	o.Response.BlockStop, o.Response.BlockReason = true, ScoutUnsaved
}
