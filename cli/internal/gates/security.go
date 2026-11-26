package gates

import (
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/scrub"
)

// Doc 06's security-scan gate: no new high or critical SAST finding, no new vulnerable
// dependency, no secret. Waivable only by the security owner.

// GosecLines keeps a linter's gosec findings, for comparing with the baseline's.
func GosecLines(out string) string {
	var b strings.Builder
	for _, l := range strings.Split(out, "\n") {
		if strings.HasSuffix(l, "(gosec)") && finding.MatchString(l) {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// manifests are dependency manifests and lockfiles: a change to one is a dependency change,
// which the vulnerability audit checks (doc 06: "on any dependency change").
var manifests = []string{"go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock"}

// DependencyChanged reports whether a changed path is a dependency manifest.
func DependencyChanged(changed []string) bool {
	return slices.ContainsFunc(changed, func(p string) bool { return slices.Contains(manifests, path.Base(p)) })
}

// MayChangeDependencies reports whether a task with these paths may change a dependency
// manifest: it names one, or holds a glob, which may match one.
func MayChangeDependencies(paths []string) bool {
	return DependencyChanged(paths) || slices.ContainsFunc(paths, func(p string) bool { return strings.ContainsAny(p, "*?[") })
}

// SecretsIn lists, by file and kind, what looks like a secret in each file's added lines.
// The value itself is never repeated.
func SecretsIn(added map[string]string) []string {
	s := scrub.New()
	var out []string
	for file, text := range added {
		if _, found := s.String(text); len(found) > 0 {
			kinds := make([]string, 0, len(found))
			for _, f := range found {
				kinds = append(kinds, f.Kind)
			}
			out = append(out, file+" ("+strings.Join(kinds, ", ")+")")
		}
	}
	sort.Strings(out)
	return out
}

// Vuln is one of govulncheck's findings.
type Vuln struct {
	ID       string
	FoundIn  string
	FixedIn  string
	Standard bool // in Go's standard library: only a newer toolchain fixes it
}

// String names the finding with where it is and what fixes it, so the developer can act on
// the gate's detail alone.
func (v Vuln) String() string {
	s := v.ID
	if v.FoundIn == "" {
		return s
	}
	s += " in " + v.FoundIn
	if v.Standard {
		s += " (the Go toolchain)"
	}
	if v.FixedIn == "" || v.FixedIn == "N/A" {
		return s + ", with no fixed version"
	}
	return s + ", fixed in " + v.FixedIn
}

// Vulnerabilities lists govulncheck's findings, in its order.
func Vulnerabilities(out string) []Vuln {
	var got []Vuln
	for _, l := range strings.Split(out, "\n") {
		t := strings.TrimSpace(l)
		if _, id, ok := strings.Cut(t, ": "); ok && strings.HasPrefix(t, "Vulnerability #") {
			got = append(got, Vuln{ID: id})
			continue
		}
		if len(got) == 0 {
			continue
		}
		v := &got[len(got)-1]
		switch {
		case t == "Standard library":
			v.Standard = true
		case strings.HasPrefix(t, "Found in: ") && v.FoundIn == "":
			v.FoundIn = strings.TrimPrefix(t, "Found in: ")
		case strings.HasPrefix(t, "Fixed in: ") && v.FixedIn == "":
			v.FixedIn = strings.TrimPrefix(t, "Fixed in: ")
		}
	}
	return got
}
