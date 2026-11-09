package investigation

import "slices"

// Confidence labels, weakest first (FR-DISC-003).
const (
	Unverified = "unverified"
	Likely     = "likely"
	Confirmed  = "confirmed"
)

// Source systems (M2 plan G8): two sources are independent only if their systems differ, so
// code plus traffic can confirm a claim and two code files cannot.
const (
	Static  = "static"
	Runtime = "runtime"
	People  = "people"
)

var systemOf = map[string]string{
	"code": Static, "schema": Static, "migration": Static, "manifest": Static, "codeowners": Static,
	"catalog": Static, "workflow": Static,
	"trace": Runtime, "metric": Runtime, "log": Runtime, "mesh": Runtime, "ingress": Runtime, "gateway": Runtime,
	"cluster": Runtime, "cloud": Runtime, "facets": Runtime, "alert": Runtime, "deploy": Runtime,
	"error_tracker": Runtime, "incident": Runtime,
	"pr": People, "branch": People, "commit": People, "ticket": People, "chat": People, "oncall": People,
}

// weak kinds never count towards independence: naming similarity, human hints and docs are the
// least reliable sources in doc 03's reliability table.
var weak = map[string]bool{"name": true, "hint": true, "docs": true}

// runtimePredicates describe runtime behaviour; doc 03 FR-DISC-002 requires a runtime source.
var runtimePredicates = map[string]bool{
	"calls": true, "consumed_by": true, "routes_via": true, "deployed_version": true, "replicas": true,
	"health": true, "reads_config": true, "mounts_secret": true,
}

// group is one mandatory source: any of its evidence kinds satisfies it.
type group struct {
	name  string
	kinds []string
}

// questionType holds doc 03's mandatory sources for the claims that answer the question.
type questionType struct {
	predicates []string
	mandatory  []group
}

// questionTypes is doc 03 §Question recipes, mandatory sources only (M2 plan G9).
var questionTypes = map[string]questionType{
	"data_owner":        {[]string{"owns_data", "writes_data"}, []group{{"schema or migrations", []string{"schema", "migration"}}, {"write paths in code", []string{"code"}}}},
	"endpoint_callers":  {[]string{"calls"}, []group{{"traces or access logs", []string{"trace", "log"}}, {"client code", []string{"code"}}}},
	"event_consumers":   {[]string{"consumed_by"}, []group{{"subscription code", []string{"code"}}, {"consumer group or subscription metrics", []string{"metric"}}}},
	"service_path":      {[]string{"routes_via", "calls"}, []group{{"client code", []string{"code"}}, {"cluster service, ingress or mesh config", []string{"cluster", "ingress", "mesh", "gateway"}}}},
	"deployed_state":    {[]string{"deployed_version", "replicas"}, []group{{"cluster or cloud API", []string{"cluster", "cloud"}}}},
	"service_health":    {[]string{"health"}, []group{{"metrics or alerts", []string{"metric", "alert"}}}},
	"service_owner":     {[]string{"owned_by"}, []group{{"CODEOWNERS or catalog", []string{"codeowners", "catalog"}}}},
	"active_work":       {[]string{"active_work"}, []group{{"open PRs and branches", []string{"pr", "branch"}}}},
	"config_dependency": {[]string{"reads_config", "mounts_secret"}, []group{{"deployment env and mounts", []string{"cluster"}}}},
	"drift_check":       {[]string{"deployed_version"}, []group{{"cluster state", []string{"cluster"}}, {"manifests in repo", []string{"manifest"}}}},
	"custom":            {},
}

// Ceiling is the highest confidence a claim's evidence allows and why.
type Ceiling struct {
	Label string `json:"label"`
	Why   string `json:"why"`
}

// CeilingOf applies M2 plan §4.3 to one claim of a question of type qtype. The first rule that
// caps the claim wins, so the reason names the main limit.
func CeilingOf(c Claim, qtype string) Ceiling {
	if slices.ContainsFunc(c.Evidence, Source.contradicts) {
		return Ceiling{Unverified, "a source contradicts the claim; report the disagreement as drift"}
	}
	kinds := make([]string, 0, len(c.Evidence))
	for _, e := range c.Evidence {
		kinds = append(kinds, e.Kind)
	}
	systems := map[string]bool{}
	strong := 0
	for _, k := range kinds {
		if !weak[k] {
			systems[systemOf[k]], strong = true, strong+1
		}
	}
	switch {
	case runtimePredicates[c.Predicate] && !systems[Runtime]:
		return Ceiling{Unverified, "a runtime claim needs a runtime source (traces, metrics, logs, cluster)"}
	case strong == 0 && slices.Contains(kinds, "hint"):
		return Ceiling{Likely, "only a human hint"}
	case strong == 0:
		return Ceiling{Unverified, "only naming similarity or docs"}
	case len(systems) < 2:
		return Ceiling{Likely, "one source system; confirmed needs two independent ones"}
	}
	if missing := missingMandatory(c, qtype); len(missing) > 0 {
		return Ceiling{Likely, "mandatory source missing: " + missing[0]}
	}
	if sampledOnly(c.Evidence) {
		return Ceiling{Likely, "runtime evidence is sampled"}
	}
	return Ceiling{Confirmed, "two independent source systems agree"}
}

// missingMandatory lists the mandatory sources a claim answering the question lacks.
func missingMandatory(c Claim, qtype string) []string {
	qt := questionTypes[qtype]
	if !slices.Contains(qt.predicates, c.Predicate) {
		return nil
	}
	var missing []string
	for _, g := range qt.mandatory {
		if !slices.ContainsFunc(c.Evidence, func(e Source) bool { return slices.Contains(g.kinds, e.Kind) }) {
			missing = append(missing, g.name)
		}
	}
	return missing
}

// sampledOnly reports whether every runtime source of the claim is sampled traffic.
func sampledOnly(evidence []Source) bool {
	runtime := 0
	for _, e := range evidence {
		if systemOf[e.Kind] != Runtime {
			continue
		}
		if e.Sampling == 0 || e.Sampling == 1 {
			return false
		}
		runtime++
	}
	return runtime > 0
}
