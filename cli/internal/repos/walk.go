package repos

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// MaxDepth is how far below a root clones are looked for (doc 03 step 2).
const MaxDepth = 4

// skip are folders that never hold a developer's clone but can be huge.
var skip = map[string]bool{
	"node_modules": true, "vendor": true, "build": true, "dist": true, "target": true, "out": true,
	".venv": true, "venv": true, "__pycache__": true, ".gradle": true, ".terraform": true, "Pods": true,
}

// Walk returns the folders under roots that contain a .git folder, sorted, and whether it
// finished before the deadline. It never follows a symlink, never descends into a clone, never
// enters a hidden folder, and reads nothing but folder listings (doc 03: file contents outside
// repos are never read). Unreadable folders are skipped.
func Walk(roots []string, deadline time.Time, now func() time.Time) ([]string, bool) {
	var found []string
	type item struct {
		dir   string
		depth int
	}
	queue := make([]item, 0, len(roots))
	for _, r := range roots {
		queue = append(queue, item{r, 0})
	}
	for len(queue) > 0 {
		if now().After(deadline) {
			slices.Sort(found)
			return found, false
		}
		it := queue[0]
		queue = queue[1:]
		if isClone(it.dir) {
			found = append(found, it.dir)
			continue
		}
		if it.depth == MaxDepth {
			continue
		}
		entries, err := os.ReadDir(it.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if !e.IsDir() || e.Type()&fs.ModeSymlink != 0 || skip[name] || name[0] == '.' {
				continue
			}
			queue = append(queue, item{filepath.Join(it.dir, name), it.depth + 1})
		}
	}
	slices.Sort(found)
	return found, true
}

// isClone reports whether dir has a .git folder (a worktree's or submodule's .git file is not a
// clone of its own).
func isClone(dir string) bool {
	info, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
