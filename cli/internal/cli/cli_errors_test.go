package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

func TestCommandsOutsideWorkspace(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"change", "list"}, {"change", "show", "CHG-001"}, {"change", "new", "--title", "t", "--prd-text", "p"},
	} {
		code, env := h.runJSON(args...)
		if code != aerr.ExitState || errCode(env) != string(aerr.StateNotInitialised) {
			t.Errorf("%v: exit %d env %v", args, code, env)
		}
	}
}

func TestInvalidConfigBlocksCommands(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@1\nbudgets:\n  default:\n    max_cost_usd: 999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, env := h.runJSON("change", "list")
	if code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyConfigLoosens) {
		t.Errorf("exit %d env %v", code, env)
	}
}

func TestNoHomeDirectory(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	h.home = "" // ATHEL_HOME empty and UserHome fails in the harness
	code, env := h.runJSON("change", "list")
	if code != aerr.ExitState || errCode(env) != string(aerr.StateIO) {
		t.Errorf("exit %d env %v", code, env)
	}
}

func TestChangeShowErrors(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	if code, env := h.runJSON("change", "show", "CHG-404"); code != aerr.ExitState || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("missing: exit %d env %v", code, env)
	}
	h.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "changes", "CHG-001", "change.yaml"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"change", "list"}, {"state", "show"}} {
		if code, env := h.runJSON(args...); code != aerr.ExitState || errCode(env) != string(aerr.StateSchema) {
			t.Errorf("%v on a corrupt change: exit %d env %v", args, code, env)
		}
	}
}

func TestJSONErrorShapes(t *testing.T) {
	joined := errors.Join(aerr.New(aerr.StateIllegalTransition, "nope"), aerr.New(aerr.StateIO, "audit write failed"))
	b, err := json.Marshal(jsonError(joined))
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); !strings.Contains(s, `"code":"E-STATE-ILLEGAL-TRANSITION"`) || !strings.Contains(s, "audit write failed") {
		t.Errorf("joined error JSON = %s", s)
	}
	b, err = json.Marshal(jsonError(fmt.Errorf("untyped")))
	if err != nil || !strings.Contains(string(b), `"code":"INTERNAL"`) {
		t.Errorf("untyped error JSON = %s %v", b, err)
	}
	if exitCode(errors.New("x")) != aerr.ExitInternal {
		t.Error("untyped error must exit internal")
	}
	if !strings.Contains(humanError(joined), "audit write failed") {
		t.Errorf("human error hides the secondary failure: %q", humanError(joined))
	}
}

func TestViewsIncludeStepLockAndBlocked(t *testing.T) {
	step := state.Step{Name: "investigate", StartedAt: "2026-11-01T10:00:00.000Z", Attempt: 2, Status: "running"}
	origin := state.Investigating
	c := &state.Change{
		ID: "CHG-001", Title: "t", State: state.Blocked, BlockedFrom: &origin, Step: &step,
		Lock:   &state.Lock{Session: "A", Since: "2026-11-01T10:00:00.000Z", HeartbeatAt: "2026-11-01T10:00:00.000Z", TTLMin: 30},
		Budget: state.Budget{Status: "warning", MaxCostUSD: 40, UsedCostUSD: 33},
	}
	sum := summarize(c)
	if *sum.Step != "investigate" || *sum.LockHolder != "A" {
		t.Errorf("summary = %+v", sum)
	}
	v := viewOf(c, time.Date(2026, 11, 1, 10, 5, 0, 0, time.UTC))
	if *v.BlockedFrom != "investigating" || v.Lock.Session != "A" || v.Lock.Stale || v.Budget.Status != "warning" ||
		strings.Join(v.AllowedNext, ",") != "investigating,cancelled" {
		t.Errorf("view = %+v", v)
	}
	b, err := json.Marshal(v)
	if err != nil || !strings.Contains(string(b), `"step":{"name":"investigate"`) {
		t.Errorf("view JSON uses change.yaml key names: %s %v", b, err)
	}
}
