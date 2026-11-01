// Package config computes Athel's effective configuration from four layers (doc 02
// §Configuration and policy files): built-in defaults, org, user and project, in that order of
// authority. A lower layer may only make a rule stricter unless a higher layer lists the key
// under `overridable:`. The merge engine is generic so M1's policy files reuse it.
package config

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Kind says what "stricter" means for one key.
type Kind int

// Merge kinds.
const (
	// Free keys are preferences with no safety meaning; any layer may set them.
	Free Kind = iota
	// Min keys are stricter when lower (budgets, TTLs).
	Min
	// Max keys are stricter when higher (thresholds such as coverage).
	Max
	// Ordered keys are enums ranked loose → strict (e.g. allow < approve < deny).
	Ordered
	// Union keys are deny-style lists: lower layers can only add items.
	Union
	// Intersect keys are allow-style lists: lower layers can only remove items.
	Intersect
)

// Rule is the merge rule of one dotted key. Order is the loose → strict ranking for Ordered.
type Rule struct {
	Kind  Kind
	Order []string
}

// Rules maps dotted keys to rules. A key ending in ".*" matches any single child key.
type Rules map[string]Rule

// overridableKey is the reserved key holding a layer's overridable list.
const overridableKey = "overridable"

// Layer is one parsed config file.
type Layer struct {
	Name string // builtin | org | user | project
	Path string
	Doc  map[string]any
}

// Effective is the merged configuration.
type Effective struct {
	Values      map[string]any    // nested, ready for JSON/YAML
	Origins     map[string]string // dotted key → layer name that set the winning value
	Overridable []string          // keys any layer marked overridable
}

func (r Rules) lookup(key string) (Rule, bool) {
	if rule, ok := r[key]; ok {
		return rule, true
	}
	if i := strings.LastIndexByte(key, '.'); i > 0 {
		rule, ok := r[key[:i]+".*"]
		return rule, ok
	}
	return Rule{}, false
}

// Merge applies layers in order of authority (highest first) and returns the effective
// config, or one E-POLICY error that lists every loosening and every unknown key.
func Merge(layers []Layer, rules Rules) (*Effective, error) {
	flat := map[string]any{}
	eff := &Effective{Origins: map[string]string{}}
	overridable := map[string]bool{}
	var problems []string
	invalid := false
	for _, l := range layers {
		leaves := map[string]any{}
		flatten("", l.Doc, leaves)
		for _, key := range slices.Sorted(maps.Keys(leaves)) {
			if key == overridableKey {
				continue
			}
			val := leaves[key]
			rule, ok := rules.lookup(key)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s (%s): unknown key %q", l.Name, l.Path, key))
				invalid = true
				continue
			}
			cur, has := flat[key]
			if !has || overridable[key] {
				flat[key], eff.Origins[key] = val, l.Name
				continue
			}
			merged, err, bad := tighten(rule, cur, val)
			if err != "" {
				invalid = invalid || bad
				problems = append(problems, fmt.Sprintf("%s (%s): %s %s; the %s layer set it", l.Name, l.Path, key, err, eff.Origins[key]))
				continue
			}
			flat[key], eff.Origins[key] = merged, l.Name
		}
		for _, k := range stringList(l.Doc[overridableKey]) {
			overridable[k] = true
		}
	}
	if len(problems) > 0 {
		code := aerr.PolicyConfigLoosens
		if invalid {
			code = aerr.PolicyConfigInvalid
		}
		return nil, aerr.New(code, "invalid configuration: "+strings.Join(problems, "; ")).
			WithNextAction("fix the named file; a lower layer may only tighten a rule unless a higher layer lists it under overridable")
	}
	eff.Values = unflatten(flat)
	eff.Overridable = slices.Sorted(maps.Keys(overridable))
	return eff, nil
}

// tighten returns the merged value, or a description of why next is rejected and whether the
// rejection is a malformed value (invalid) rather than a loosening.
func tighten(rule Rule, cur, next any) (merged any, problem string, invalid bool) {
	switch rule.Kind {
	case Min, Max:
		c, okC := number(cur)
		n, okN := number(next)
		if !okC || !okN {
			return nil, fmt.Sprintf("is not a number (%v)", next), true
		}
		if (rule.Kind == Min && n > c) || (rule.Kind == Max && n < c) {
			return nil, fmt.Sprintf("= %v is looser than %v", next, cur), false
		}
	case Ordered:
		ci, ni := slices.Index(rule.Order, fmt.Sprint(cur)), slices.Index(rule.Order, fmt.Sprint(next))
		if ni < 0 {
			return nil, fmt.Sprintf("is not a known value (%v)", next), true
		}
		if ni < ci {
			return nil, fmt.Sprintf("= %v is looser than %v", next, cur), false
		}
	case Union:
		return union(stringList(cur), stringList(next)), "", false
	case Intersect:
		allowed := stringList(cur)
		for _, item := range stringList(next) {
			if !slices.Contains(allowed, item) {
				return nil, fmt.Sprintf("adds %q, which is looser than %v", item, cur), false
			}
		}
	}
	return next, "", false
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

func union(a, b []string) []any {
	seen := map[string]bool{}
	var out []any
	for _, s := range append(slices.Clone(a), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// flatten turns nested maps into dotted leaf keys; lists and scalars are leaves.
func flatten(prefix string, m map[string]any, out map[string]any) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if child, ok := v.(map[string]any); ok {
			flatten(key, child, out)
			continue
		}
		out[key] = v
	}
}

func unflatten(flat map[string]any) map[string]any {
	out := map[string]any{}
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.Split(key, ".")
		node := out
		for _, p := range parts[:len(parts)-1] {
			next, ok := node[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				node[p] = next
			}
			node = next
		}
		node[parts[len(parts)-1]] = flat[key]
	}
	return out
}
