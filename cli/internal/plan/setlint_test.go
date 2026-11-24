package plan

import (
	"strings"
	"testing"
)

func kt(id, repo, kind string, deps ...string) Task {
	return Task{ID: id, Repo: repo, Kind: kind, DependsOn: deps}
}

func TestSetRules(t *testing.T) {
	v2 := Contract{Item: "GET /v2/users/{id}", Provider: "users", Change: "additive", Consumers: []string{"orders", "gateway"}}
	rename := Contract{Item: "users.phone", Provider: "users", Change: "breaking", Consumers: []string{"orders"}}
	cleanup := Task{ID: "T-09", Repo: "users", Kind: KindContract, Deferred: true}
	tests := []struct {
		name        string
		tasks       []Task
		contracts   []Contract
		coordinated bool
		want        []string
	}{
		{"every consumer moves after users expands", []Task{
			kt("T-01", "users", KindExpand), kt("T-02", "orders", KindConsume, "T-01"), kt("T-03", "gateway", KindConsume, "T-01"),
		}, []Contract{v2}, false, nil},
		{"a consumer in the set without a task", []Task{
			kt("T-01", "users", KindExpand), kt("T-02", "orders", KindConsume, "T-01"),
		}, []Contract{v2}, false, []string{"gateway consumes GET /v2/users/{id} from users and is in the change set, but no task changes it"}},
		{"a consumer after another repo's expand", []Task{
			kt("T-01", "users", KindExpand), kt("T-02", "payments", KindExpand),
			kt("T-03", "orders", KindConsume, "T-02"), kt("T-04", "gateway", KindConsume, "T-01"),
		}, []Contract{v2}, false, []string{"T-03 moves orders to GET /v2/users/{id} but depends on no expand task in users"}},
		{"a breaking change with an expand path", []Task{
			kt("T-01", "users", KindExpand), kt("T-02", "orders", KindConsume, "T-01"), cleanup,
		}, []Contract{rename}, false, nil},
		{"a breaking change without one", []Task{
			kt("T-01", "users", KindInternal), kt("T-02", "orders", KindInternal, "T-01"),
		}, []Contract{rename}, false, []string{"users.phone is a breaking change for orders; plan it as expand in users"}},
		{"a coordinated release needs no expand path", []Task{
			kt("T-01", "users", KindInternal), kt("T-02", "orders", KindInternal, "T-01"),
		}, []Contract{rename}, true, nil},
		{"internal changes have no consumers to check", []Task{kt("T-01", "users", KindInternal)},
			[]Contract{{Item: "x", Provider: "users", Change: "internal", Consumers: []string{"orders"}}}, false, nil},
		{"a consumer inside the provider's own repo", []Task{kt("T-01", "users", KindConsume)},
			[]Contract{{Item: "x", Provider: "users", Change: "additive", Consumers: []string{"users"}}}, false, nil},
		{"a deferred task in the consumer is no task", []Task{
			kt("T-01", "users", KindExpand), {ID: "T-02", Repo: "orders", Kind: KindContract, Deferred: true},
		}, []Contract{{Item: "x", Provider: "users", Change: "additive", Consumers: []string{"orders"}}}, false, []string{"orders consumes x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setRules(&Plan{Tasks: tt.tasks}, LintContext{Contracts: tt.contracts, Coordinated: tt.coordinated})
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %d problems like %q", got, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if !strings.HasPrefix(got[i], w) {
					t.Errorf("problem %d = %q, want prefix %q", i, got[i], w)
				}
			}
		})
	}
}

func TestSetRulesSameRepo(t *testing.T) {
	alias := func(service, repo string) bool { return service == repo || service == "user-svc" && repo == "users" }
	got := setRules(&Plan{Tasks: []Task{kt("T-01", "users", KindExpand), kt("T-02", "orders", KindConsume, "T-01")}},
		LintContext{SameRepo: alias, Contracts: []Contract{{Item: "x", Provider: "user-svc", Change: "additive", Consumers: []string{"orders"}}}})
	if len(got) != 0 {
		t.Errorf("a service named differently from its repo: %q", got)
	}
}

func TestJoinAnd(t *testing.T) {
	for in, want := range map[string]string{"": "", "a": "a", "a,b": "a and b", "a,b,c": "a, b and c"} {
		var items []string
		if in != "" {
			items = strings.Split(in, ",")
		}
		if got := joinAnd(items); got != want {
			t.Errorf("joinAnd(%q) = %q", in, got)
		}
	}
}
