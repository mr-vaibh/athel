// Package repos finds the developer's local clones under confirmed search roots and maps each
// to its remote (doc 03 §Repo access across services; FR-DISC-015). Clones are only ever read:
// remotes and branches come from files inside .git, the last commit from read-only git log.
package repos

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultRoots are the folders doc 03 proposes at setup; the developer confirms or edits them
// once, and may add a company folder.
var DefaultRoots = []string{"~/Developer", "~/Developers", "~/Projects", "~/code", "~/src", "~/work", "~/Documents", "~/Desktop"}

// Expand turns a configured root (absolute or ~/…) into an absolute path.
func Expand(root, home string) string {
	if rest, ok := strings.CutPrefix(root, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return filepath.Clean(root)
}

// Propose returns the default roots that exist and are not already confirmed. It only checks
// that each folder exists; nothing inside is read.
func Propose(current []string, home string) []string {
	out := []string{}
	for _, r := range DefaultRoots {
		if contains(current, r) {
			continue
		}
		if info, err := os.Stat(Expand(r, home)); err == nil && info.IsDir() {
			out = append(out, r)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
