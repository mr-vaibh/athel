package investigation

import (
	"os"
	"strings"
	"testing"
)

func ev(kinds ...string) []Source {
	out := make([]Source, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, Source{Kind: k, Ref: k + "-ref"})
	}
	return out
}

func claim(predicate string, evidence []Source) Claim {
	return Claim{ID: "C1", Predicate: predicate, Evidence: evidence}
}

func TestCeiling(t *testing.T) {
	sampled := ev("code", "trace")
	sampled[1].Sampling = 0.01
	mixed := ev("code", "trace", "metric")
	mixed[1].Sampling = 0.01
	tests := []struct {
		name  string
		qtype string
		c     Claim
		want  string
		why   string
	}{
		{"code plus traffic confirms", "event_consumers", claim("consumed_by", ev("code", "metric")), Confirmed, "two independent"},
		{"two code files never confirm", "custom", claim("owned_by", ev("code", "code")), Likely, "one source system"},
		{"two runtime kinds are one system", "custom", claim("calls", ev("trace", "metric")), Likely, "one source system"},
		{"name only", "custom", claim("alias_of", ev("name")), Unverified, "naming similarity"},
		{"docs only", "custom", claim("owned_by", ev("docs")), Unverified, "naming similarity or docs"},
		{"name plus one strong", "custom", claim("alias_of", ev("name", "cluster")), Likely, "one source system"},
		{"name cannot make a second system", "custom", claim("alias_of", ev("name", "cluster", "hint")), Likely, "one source system"},
		{"hint only", "custom", claim("owned_by", ev("hint")), Likely, "human hint"},
		{"runtime claim from code only", "custom", claim("calls", ev("code", "pr")), Unverified, "runtime source"},
		{"runtime claim from name only", "custom", claim("consumed_by", ev("name")), Unverified, "runtime source"},
		{"mandatory source missing", "endpoint_callers", claim("calls", ev("code", "metric")), Likely, "traces or access logs"},
		{"mandatory sources present", "endpoint_callers", claim("calls", ev("code", "log")), Confirmed, "two independent"},
		{"mandatory rule only for answering claims", "endpoint_callers", claim("owned_by", ev("codeowners", "commit")), Confirmed, "two independent"},
		{"sampled traffic only", "endpoint_callers", claim("calls", sampled), Likely, "sampled"},
		{"sampled plus unsampled runtime", "endpoint_callers", claim("calls", mixed), Confirmed, "two independent"},
		{"people plus static", "service_owner", claim("owned_by", ev("codeowners", "commit")), Confirmed, "two independent"},
		{"deployed state needs the cluster", "deployed_state", claim("deployed_version", ev("manifest", "facets")), Likely, "cluster or cloud API"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CeilingOf(tt.c, tt.qtype)
			if got.Label != tt.want || !strings.Contains(got.Why, tt.why) {
				t.Errorf("ceiling = %+v, want %s (%s)", got, tt.want, tt.why)
			}
		})
	}
}

func TestTenQuestionTypesMandatorySources(t *testing.T) {
	// One satisfying evidence set per standard type: each must be able to reach confirmed, and
	// losing any mandatory group must cap the claim at likely.
	tests := map[string]struct {
		predicate string
		evidence  []string
	}{
		"data_owner":        {"owns_data", []string{"migration", "code", "commit"}},
		"endpoint_callers":  {"calls", []string{"trace", "code"}},
		"event_consumers":   {"consumed_by", []string{"code", "metric"}},
		"service_path":      {"routes_via", []string{"code", "ingress"}},
		"deployed_state":    {"deployed_version", []string{"cluster", "manifest"}},
		"service_health":    {"health", []string{"metric", "pr"}},
		"service_owner":     {"owned_by", []string{"codeowners", "commit"}},
		"active_work":       {"active_work", []string{"pr", "code"}},
		"config_dependency": {"mounts_secret", []string{"cluster", "manifest"}},
		"drift_check":       {"deployed_version", []string{"cluster", "manifest"}},
	}
	if len(tests) != len(questionTypes)-1 {
		t.Fatalf("%d types tested, %d defined (plus custom)", len(tests), len(questionTypes)-1)
	}
	for qtype, tt := range tests {
		t.Run(qtype, func(t *testing.T) {
			if got := CeilingOf(claim(tt.predicate, ev(tt.evidence...)), qtype); got.Label != Confirmed {
				t.Fatalf("full evidence: %+v", got)
			}
			for _, g := range questionTypes[qtype].mandatory {
				var kept []string
				for _, k := range tt.evidence {
					if !containsStr(g.kinds, k) {
						kept = append(kept, k)
					}
				}
				if got := CeilingOf(claim(tt.predicate, ev(kept...)), qtype); got.Label == Confirmed {
					t.Errorf("without %s still confirmed (%v)", g.name, kept)
				}
			}
		})
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestScoutPromptListsMandatorySources keeps the scout's table of mandatory sources in step with
// the ceilings the CLI enforces: the scout cannot gather what it is never told about.
func TestScoutPromptListsMandatorySources(t *testing.T) {
	b, err := os.ReadFile("../../../plugin/agents/scout.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		cells := strings.Split(strings.TrimSpace(line), "|")
		if len(cells) == 4 {
			rows[strings.TrimSpace(cells[1])] = strings.TrimSpace(cells[2])
		}
	}
	for typ, qt := range questionTypes {
		if typ == "custom" {
			continue
		}
		names := make([]string, 0, len(qt.mandatory))
		for _, g := range qt.mandatory {
			names = append(names, g.name)
		}
		if want := strings.Join(names, "; "); rows[typ] != want {
			t.Errorf("scout.md row for %s is %q, the CLI requires %q", typ, rows[typ], want)
		}
	}
}
