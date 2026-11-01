package hooks

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/hookio"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

func start(d Deps) Outcome {
	return Handle(&hookio.Request{Event: hookio.SessionStart, SessionID: sess, Source: "startup"}, d)
}

func TestSessionStartSummaries(t *testing.T) {
	d, _ := newDeps(t)
	if ctx := start(d).Response.AdditionalContext; !strings.Contains(ctx, "no active changes") {
		t.Errorf("empty workspace: %q", ctx)
	}
	a := newChange(t, d, "Phone update")
	newChange(t, d, "Locked elsewhere")
	lockFor(t, d, "CHG-002", "sess-B")
	if _, err := d.Store.AddCheckpoint(a.ID, sess, "start.gaps", nil, state.TriggerManual); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Store.EnterStep(a.ID, sess, "start", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Store.RecordUsage(a.ID, sess, state.Usage{CostUSD: 35}); err != nil {
		t.Fatal(err)
	}
	ctx := start(d).Response.AdditionalContext
	for _, want := range []string{"2 active change(s)", `CHG-001 "Phone update": draft, step start running, last checkpoint start.gaps, budget warning`,
		"CHG-002", "locked by another session (sess-B)", "/athel:resume"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context missing %q:\n%s", want, ctx)
		}
	}
}

func TestSessionStartCapsSummaries(t *testing.T) {
	d, _ := newDeps(t)
	for range maxSummaries + 2 {
		newChange(t, d, "x")
	}
	ctx := start(d).Response.AdditionalContext
	if !strings.Contains(ctx, "and 2 more") || strings.Contains(ctx, "CHG-011") {
		t.Errorf("not capped:\n%s", ctx)
	}
}

func TestSessionStartExportsSessionAndReportsProblems(t *testing.T) {
	d, _ := newDeps(t)
	var path, line string
	d.EnvFile = "/env"
	d.AppendEnv = func(p, l string) error { path, line = p, l; return nil }
	if o := start(d); o.Result != "ok" || path != "/env" || line != "export ATHEL_SESSION_ID="+sess {
		t.Errorf("export: %q %q %+v", path, line, o)
	}
	d.AppendEnv = func(string, string) error { return errors.New("read-only") }
	if o := start(d); o.Result != "error" {
		t.Errorf("failed export not recorded: %+v", o)
	}
	d.AppendEnv = nil
	d.ConfigErr = errors.New("budget loosened")
	if o := start(d); !strings.Contains(o.Response.AdditionalContext, "CONFIGURATION INVALID") || o.Result != "error" {
		t.Errorf("config error: %+v", o)
	}
	if o := start(Deps{}); !strings.Contains(o.Response.AdditionalContext, "athel init") {
		t.Errorf("no workspace: %q", o.Response.AdditionalContext)
	}
	d.ConfigErr = nil
	if err := os.RemoveAll(d.Store.WS.ChangesDir()); err != nil {
		t.Fatal(err)
	}
	if o := start(d); o.Result != "error" || !strings.Contains(o.Response.AdditionalContext, "could not read changes") {
		t.Errorf("unreadable changes: %+v", o)
	}
}

func TestSessionStartVersionCheck(t *testing.T) {
	d := Deps{MinClaudeCode: "2.1.308"}
	tests := []struct {
		name    string
		version string
		err     error
		want    string
	}{
		{"same", "2.1.308 (Claude Code)", nil, ""},
		{"newer", "2.2.0", nil, ""},
		{"older", "2.1.250 (Claude Code)", nil, "WARNING Claude Code 2.1.250 (Claude Code) is older"},
		{"unknown", "", errors.New("not found"), "version unknown (not found)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d.ClaudeVersion = func() (string, error) { return tt.version, tt.err }
			got := strings.Join(versionLine(d), "")
			if (tt.want == "" && got != "") || !strings.Contains(got, tt.want) {
				t.Errorf("versionLine = %q, want %q", got, tt.want)
			}
		})
	}
	if versionLine(Deps{}) != nil {
		t.Error("version check without a probe")
	}
}

func TestOlder(t *testing.T) {
	tests := []struct {
		running, minimum string
		want             bool
	}{
		{"2.1.308", "2.1.308", false},
		{"2.1.309", "2.1.308", false},
		{"2.1.30", "2.1.308", true},
		{"3.0", "2.1.308", false},
		{"2.1", "2.1.0", false},
		{"garbage", "2.1.308", true},
		{"2.1.308", "", true},
		{"2.x.1", "2.1.0", true},
	}
	for _, tt := range tests {
		if got := Older(tt.running, tt.minimum); got != tt.want {
			t.Errorf("Older(%q, %q) = %v", tt.running, tt.minimum, got)
		}
	}
}
