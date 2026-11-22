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

// Vulnerabilities lists govulncheck's findings, one line each.
func Vulnerabilities(out string) []string {
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "Vulnerability #") {
			got = append(got, strings.TrimSpace(l))
		}
	}
	return got
}
