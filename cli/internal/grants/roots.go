package grants

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/digest"
)

// TargetRoots is the grant target for confirming repo search roots (M2 plan Q7). Search roots
// widen what Athel may read without asking (doc 04 rule 12's exception), so only the developer's
// own literal `/athel:roots <path>…` prompt can allow `athel repos roots set` with those paths.
const TargetRoots = "search_roots"

var (
	rootsPromptRe = regexp.MustCompile(`^\s*/athel:roots((?:[ \t]+\S+)+)[ \t]*$`)
	// rootRe keeps roots to characters that stay literal inside single quotes in any shell.
	rootRe = regexp.MustCompile(`^(/|~/)[A-Za-z0-9._@+,=/-]*[A-Za-z0-9._@+,=-]$`)
)

// ParseRootsPrompt returns the roots of a literal `/athel:roots` prompt, cleaned and sorted.
// Every word must be an absolute or home-relative folder; anything else is not a roots prompt.
func ParseRootsPrompt(prompt string) ([]string, bool) {
	m := rootsPromptRe.FindStringSubmatch(prompt)
	if m == nil {
		return nil, false
	}
	roots := CleanRoots(strings.Fields(m[1]))
	for _, r := range roots {
		if !rootRe.MatchString(r) {
			return nil, false
		}
	}
	return roots, true
}

// CleanRoots drops trailing slashes and duplicates and sorts, so the same folders always give
// the same list and the same grant key.
func CleanRoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if c := strings.TrimRight(r, "/"); c != "" {
			out = append(out, c)
		} else {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// RootsCommand is the exact command that saves roots, each root single-quoted so the shell
// passes "~" through unexpanded and the policy check sees the same words the developer typed.
func RootsCommand(roots []string) string {
	quoted := make([]string, 0, len(roots))
	for _, r := range CleanRoots(roots) {
		quoted = append(quoted, "'"+r+"'")
	}
	return "athel repos roots set " + strings.Join(quoted, " ")
}

// RootsKey names a set of roots in the grant book (the grant's change field).
func RootsKey(roots []string) string {
	return "ROOTS-" + digest.Bytes([]byte(strings.Join(CleanRoots(roots), "\n")))[7:19]
}
