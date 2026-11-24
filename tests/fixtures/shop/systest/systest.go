// Package systest runs the shop's system check (M5-plan §5.9, ADR 036): the gateway repo's
// systest program builds users, orders and gateway from the folders it is given, starts them
// together, and runs the cross-service flows in gateway/systest/flows.json. Athel's merge-order
// rehearsal runs the same program as the workspace's system check; the fixture's tests and the
// M5 eval grader run it through Run.
package systest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Result is one flow's outcome; Got says what came back when it failed.
type Result struct {
	Flow string `json:"flow"`
	OK   bool   `json:"ok"`
	Got  string `json:"got,omitempty"`
}

// Command is the system check's argv, with each {repo} word standing for that repo's folder:
// the form .athel/config/system.yaml holds.
var Command = []string{"go", "-C", "{gateway}", "run", "./systest", "--flows", "systest/flows.json",
	"--users", "{users}", "--orders", "{orders}", "--gateway", "{gateway}"}

// Run runs the system check on the service folders (users, orders, gateway). A failed flow is a
// Result with OK false; an error means the check could not run.
func Run(ctx context.Context, dirs map[string]string) ([]Result, error) {
	argv := make([]string, len(Command))
	for i, a := range Command {
		argv[i] = a
		if strings.HasPrefix(a, "{") {
			argv[i] = dirs[strings.Trim(a, "{}")]
		}
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the fixture's own system check
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var doc struct {
		Results []Result `json:"results"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &doc); jerr != nil {
		return nil, fmt.Errorf("system check: %w: %s", errors.Join(err, jerr), strings.TrimSpace(errOut.String()))
	}
	return doc.Results, nil
}

// Failed lists the flows that did not pass.
func Failed(results []Result) []string {
	var out []string
	for _, r := range results {
		if !r.OK {
			out = append(out, r.Flow+": "+r.Got)
		}
	}
	return out
}
