package config

import (
	"reflect"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

var testRules = Rules{
	"budget.max":    {Kind: Min},
	"coverage.min":  {Kind: Max},
	"decision":      {Kind: Ordered, Order: []string{"allow", "approve", "deny"}},
	"deny_paths":    {Kind: Union},
	"allowed_hosts": {Kind: Intersect},
	"model":         {Kind: Free},
	"features.*":    {Kind: Free},
}

func layer(name string, doc map[string]any) Layer {
	return Layer{Name: name, Path: name + ".yaml", Doc: doc}
}

func TestMergeKinds(t *testing.T) {
	tests := []struct {
		name    string
		high    map[string]any
		low     map[string]any
		want    any
		key     string
		wantErr aerr.Code
	}{
		{"min tighten", map[string]any{"budget": map[string]any{"max": 40}}, map[string]any{"budget": map[string]any{"max": 20}}, 20, "budget.max", ""},
		{"min equal", map[string]any{"budget": map[string]any{"max": 40}}, map[string]any{"budget": map[string]any{"max": 40.0}}, 40.0, "budget.max", ""},
		{"min loosen", map[string]any{"budget": map[string]any{"max": 40}}, map[string]any{"budget": map[string]any{"max": 80}}, nil, "budget.max", aerr.PolicyConfigLoosens},
		{"min not a number", map[string]any{"budget": map[string]any{"max": 40}}, map[string]any{"budget": map[string]any{"max": "x"}}, nil, "budget.max", aerr.PolicyConfigInvalid},
		{"max tighten", map[string]any{"coverage": map[string]any{"min": 90}}, map[string]any{"coverage": map[string]any{"min": 95}}, 95, "coverage.min", ""},
		{"max loosen", map[string]any{"coverage": map[string]any{"min": 90}}, map[string]any{"coverage": map[string]any{"min": int64(80)}}, nil, "coverage.min", aerr.PolicyConfigLoosens},
		{"ordered tighten", map[string]any{"decision": "approve"}, map[string]any{"decision": "deny"}, "deny", "decision", ""},
		{"ordered loosen", map[string]any{"decision": "approve"}, map[string]any{"decision": "allow"}, nil, "decision", aerr.PolicyConfigLoosens},
		{"ordered unknown", map[string]any{"decision": "approve"}, map[string]any{"decision": "maybe"}, nil, "decision", aerr.PolicyConfigInvalid},
		{"union adds", map[string]any{"deny_paths": []any{"~/.ssh"}}, map[string]any{"deny_paths": []any{".env", "~/.ssh"}}, []any{"~/.ssh", ".env"}, "deny_paths", ""},
		{"intersect narrows", map[string]any{"allowed_hosts": []any{"github", "gitlab"}}, map[string]any{"allowed_hosts": []any{"github"}}, []any{"github"}, "allowed_hosts", ""},
		{"intersect widens", map[string]any{"allowed_hosts": []any{"github"}}, map[string]any{"allowed_hosts": []any{"github", "bitbucket"}}, nil, "allowed_hosts", aerr.PolicyConfigLoosens},
		{"free replaces", map[string]any{"model": "strong"}, map[string]any{"model": "fast"}, "fast", "model", ""},
		{"wildcard free", map[string]any{"features": map[string]any{"x": true}}, map[string]any{"features": map[string]any{"x": false}}, false, "features.x", ""},
		{"first setter wins freely", map[string]any{}, map[string]any{"budget": map[string]any{"max": 999}}, 999, "budget.max", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eff, err := Merge([]Layer{layer("org", tt.high), layer("user", tt.low)}, testRules)
			if tt.wantErr != "" {
				if !aerr.HasCode(err, tt.wantErr) {
					t.Fatalf("err = %v, want %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := get(eff.Values, tt.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}

func get(m map[string]any, dotted string) any {
	var cur any = m
	for _, p := range strings.Split(dotted, ".") {
		cur = cur.(map[string]any)[p]
	}
	return cur
}

func TestOverridableAllowsLoosen(t *testing.T) {
	org := layer("org", map[string]any{"overridable": []any{"budget.max"}, "budget": map[string]any{"max": 40}, "decision": "deny"})
	user := layer("user", map[string]any{"budget": map[string]any{"max": 100}})
	eff, err := Merge([]Layer{org, user}, testRules)
	if err != nil {
		t.Fatal(err)
	}
	if get(eff.Values, "budget.max") != 100 || eff.Origins["budget.max"] != "user" || !reflect.DeepEqual(eff.Overridable, []string{"budget.max"}) {
		t.Errorf("eff = %+v", eff)
	}
	selfMarked := layer("user", map[string]any{"overridable": []any{"decision"}, "decision": "allow"})
	if _, err := Merge([]Layer{org, selfMarked}, testRules); !aerr.HasCode(err, aerr.PolicyConfigLoosens) {
		t.Errorf("a layer cannot unlock its own loosening: err = %v", err)
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	_, err := Merge([]Layer{layer("project", map[string]any{"budgte": map[string]any{"max": 1}})}, testRules)
	if !aerr.HasCode(err, aerr.PolicyConfigInvalid) || !strings.Contains(err.Error(), `"budgte.max"`) {
		t.Errorf("err = %v", err)
	}
}

func TestErrorNamesLayerAndKey(t *testing.T) {
	layers := []Layer{
		layer("builtin", map[string]any{"budget": map[string]any{"max": 40}, "decision": "approve"}),
		layer("org", map[string]any{"budget": map[string]any{"max": 30}}),
		layer("project", map[string]any{"budget": map[string]any{"max": 35}, "decision": "allow"}),
	}
	_, err := Merge(layers, testRules)
	msg := err.Error()
	for _, want := range []string{"project (project.yaml): budget.max = 35 is looser than 30; the org layer set it",
		"project (project.yaml): decision = allow is looser than approve; the builtin layer set it"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not contain %q", msg, want)
		}
	}
}

func TestOriginsAndNesting(t *testing.T) {
	eff, err := Merge([]Layer{
		layer("builtin", map[string]any{"budget": map[string]any{"max": 40}, "model": "strong"}),
		layer("user", map[string]any{"model": "fast"}),
	}, testRules)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"budget": map[string]any{"max": 40}, "model": "fast"}
	if !reflect.DeepEqual(eff.Values, want) {
		t.Errorf("values = %#v", eff.Values)
	}
	if eff.Origins["budget.max"] != "builtin" || eff.Origins["model"] != "user" {
		t.Errorf("origins = %v", eff.Origins)
	}
	empty, err := Merge(nil, testRules)
	if err != nil || len(empty.Values) != 0 || len(empty.Overridable) != 0 {
		t.Errorf("no layers: %+v %v", empty, err)
	}
}
