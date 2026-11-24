package changeset

import (
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/plan"
)

func threeRepoSet(t *testing.T, coordinated bool) *Set {
	t.Helper()
	s, err := Derive(&plan.Plan{Tasks: []plan.Task{
		task("T-01", "users", plan.KindExpand),
		task("T-02", "orders", plan.KindConsume, "T-01"),
		task("T-03", "gateway", plan.KindConsume, "T-01"),
		deferred(task("T-04", "users", plan.KindContract, "T-02", "T-03")),
	}}, coordinated)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func pr(repo, state, mergedAt string) PR {
	return PR{Ref: "acme/" + repo + "#9", State: state, MergedAt: mergedAt}
}

func TestStatusNextToMerge(t *testing.T) {
	s := threeRepoSet(t, false)
	tests := []struct {
		name  string
		now   map[string]PR
		next  string
		clean string
	}{
		{"the provider first", map[string]PR{"users": pr("users", "ready", ""), "orders": pr("orders", "ready", ""), "gateway": pr("gateway", "ready", "")},
			"acme/users#9", "waits until every PR"},
		{"then both consumers", map[string]PR{"users": pr("users", "merged", "2026-11-24T10:00:00Z"), "orders": pr("orders", "ready", ""), "gateway": pr("gateway", "ready", "")},
			"acme/gateway#9,acme/orders#9", "waits until every PR"},
		{"a draft is not next", map[string]PR{"users": pr("users", "draft", ""), "orders": pr("orders", "draft", "")}, "", "waits"},
		{"all merged: the cleanup", map[string]PR{"users": pr("users", "merged", "1"), "orders": pr("orders", "merged", "2"), "gateway": pr("gateway", "merged", "3")},
			"", "run /athel:followup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := s.Status(tt.now)
			if strings.Join(r.Next, ",") != tt.next || len(r.Cleanup) != 1 || !strings.Contains(r.Cleanup[0], tt.clean) || len(r.Alarms) != 0 {
				t.Errorf("next %v alarms %v cleanup %v", r.Next, r.Alarms, r.Cleanup)
			}
			if len(r.PRs) != 3 || r.PRs[0].Repo != "users" || r.PRs[0].Order != 1 {
				t.Errorf("PRs: %+v", r.PRs)
			}
		})
	}
}

func TestOutOfOrderMergeAlarm(t *testing.T) {
	s := threeRepoSet(t, false)
	r := s.Status(map[string]PR{"users": pr("users", "ready", ""), "orders": pr("orders", "merged", "2026-11-24T10:00:00Z"), "gateway": pr("gateway", "ready", "")})
	if len(r.Alarms) != 1 || !strings.HasPrefix(r.Alarms[0], "acme/orders#9 was merged before acme/users#9, which it must follow: merge acme/users#9 now") {
		t.Errorf("alarm: %v", r.Alarms)
	}
	r = s.Status(map[string]PR{"users": pr("users", "merged", "2026-11-24T11:00:00Z"), "orders": pr("orders", "merged", "2026-11-24T10:00:00Z"), "gateway": pr("gateway", "ready", "")})
	if len(r.Alarms) != 1 || !strings.Contains(r.Alarms[0], "check what was deployed in between") {
		t.Errorf("a late provider merge: %v", r.Alarms)
	}
}

func TestCoordinatedStatus(t *testing.T) {
	s := threeRepoSet(t, true)
	r := s.Status(map[string]PR{"users": pr("users", "merged", "1"), "orders": pr("orders", "ready", ""), "gateway": {}})
	if len(r.Alarms) != 1 || !strings.HasPrefix(r.Alarms[0], "coordinated release partly merged (acme/users#9): merge gateway, acme/orders#9") {
		t.Errorf("partly merged: %v %v", r.Next, r.Alarms)
	}
	r = s.Status(map[string]PR{"users": pr("users", "merged", "1"), "orders": pr("orders", "merged", "2"), "gateway": pr("gateway", "merged", "3")})
	if len(r.Next) != 0 || len(r.Alarms) != 0 {
		t.Errorf("all merged: %v %v", r.Next, r.Alarms)
	}
}
