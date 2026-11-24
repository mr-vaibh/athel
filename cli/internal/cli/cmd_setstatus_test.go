package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/build"
	"github.com/mr-vaibh/athel/cli/internal/changeset"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/plan"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// hostGH answers gh pr view for the users PR (901, open, approved, green) and the orders PR
// (777, merged) as GitHub would.
func hostGH(t *testing.T, h *harness) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1 $2 $3\" in\n" +
		"  'pr view 901') echo '{\"state\":\"OPEN\",\"isDraft\":false,\"reviewDecision\":\"APPROVED\",\"mergedAt\":\"\",\"statusCheckRollup\":[{\"status\":\"COMPLETED\",\"conclusion\":\"SUCCESS\"}]}' ;;\n" +
		"  'pr view 777') echo '{\"state\":\"MERGED\",\"isDraft\":false,\"reviewDecision\":\"APPROVED\",\"mergedAt\":\"2026-11-24T10:00:00Z\",\"statusCheckRollup\":[]}' ;;\n" +
		"  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

// TestOutOfOrderMergeAlarmThroughTheHost (M5-plan G8): orders was merged before users, which it
// must follow; status says so and what to do, and names users as the next to merge.
func TestOutOfOrderMergeAlarmThroughTheHost(t *testing.T) {
	h, _, _, _ := building(t)
	recordingGH(t, h)
	h.mustOK("git", "push", "CHG-001", "users")
	h.mustOK("pr", "open", "CHG-001", "users")
	asSet(t, h)
	hostGH(t, h)
	v := h.mustOK("set", "status", "CHG-001")
	text := asJSON(t, v)
	for _, want := range []string{
		`"source":"host"`, `"next":["acme/users#901"]`,
		"acme/orders#777 was merged before acme/users#901, which it must follow: merge acme/users#901 now",
		`"review":"APPROVED"`, `"ci":"success"`, `"state":"merged"`,
		"has not been rehearsed",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("status lacks %q:\n%s", want, text)
		}
	}
	failingGH(t, h)
	v = h.mustOK("set", "status", "CHG-001")
	if v["source"] != "recorded" || !strings.Contains(v["note"].(string), "the host could not be read") {
		t.Errorf("host down: %v", v)
	}
}

func failingGH(t *testing.T, h *harness) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\necho 'HTTP 502' >&2\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.env["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
}

func TestSetStatusWithoutASet(t *testing.T) {
	h := setup(t)
	if code, env := h.runJSON("set", "status", "CHG-001"); code != aerr.ExitState || !strings.Contains(asJSON(t, env), "no change set yet") {
		t.Errorf("no set: %v", env)
	}
}

func TestCIAndStateOfAPR(t *testing.T) {
	check := func(status, conclusion string) ghPRView {
		v := ghPRView{State: "OPEN"}
		v.Checks = append(v.Checks, struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{status, conclusion})
		return v
	}
	tests := []struct {
		v         ghPRView
		ci, state string
	}{
		{ghPRView{State: "OPEN"}, "none", "ready"},
		{ghPRView{State: "OPEN", IsDraft: true}, "none", "draft"},
		{ghPRView{State: "CLOSED"}, "none", "closed"},
		{check("COMPLETED", "SUCCESS"), "success", "ready"},
		{check("IN_PROGRESS", ""), "pending", "ready"},
		{check("COMPLETED", "FAILURE"), "failure", "ready"},
	}
	for _, tt := range tests {
		if ci, st := ciOf(tt.v), prState(tt.v); ci != tt.ci || st != tt.state {
			t.Errorf("%+v: ci %s state %s, want %s %s", tt.v, ci, st, tt.ci, tt.state)
		}
	}
}

func TestDroppedAndBuildMinutes(t *testing.T) {
	url, done := "https://github.com/acme/gateway/pull/5", "2026-11-24T10:30:00.000Z"
	c := &state.Change{ID: "CHG-001",
		Repos:     []state.RepoWork{{Name: "users"}, {Name: "gateway", Remote: "github.com/acme/gateway", PR: &url}},
		ChangeSet: &changeset.Set{Repos: []changeset.Entry{{Repo: "users", Order: 1}}},
		Build: []build.Task{{ID: "T-01", Status: build.Done, Started: "2026-11-24T10:00:00.000Z", DoneAt: &done},
			{ID: "T-02", Status: build.InProgress, Started: "2026-11-24T10:00:00.000Z"}}}
	if got := dropped(c); len(got) != 1 || !strings.HasPrefix(got[0], "acme/gateway#5: dropped from CHG-001") {
		t.Errorf("dropped = %v", got)
	}
	p := &plan.Plan{Tasks: []plan.Task{{ID: "T-01", Repo: "users"}, {ID: "T-02", Repo: "users"}}}
	if got := buildMinutes(c, p); got["users"] != 30 {
		t.Errorf("minutes = %v", got)
	}
	if got := buildMinutes(c, nil); len(got) != 0 {
		t.Errorf("no plan, no times: %v", got)
	}
}
