package shop

import (
	"fmt"
	"os"
	"path/filepath"
)

// SystemServices are the Go services that run together for M5's system check (M5-plan §5.9).
var SystemServices = []string{"users", "orders", "gateway"}

// SystemDir holds the system layer: seed/<repo>/ makes the services runnable side by side (each
// reads its address and its peers' from the environment), flows.yaml holds the cross-service
// flows, and reference/<step>/<repo>/ are known-good changes for the fixture's own tests.
func SystemDir() string { return filepath.Join(filepath.Dir(Sources()), "system") }

// SystemTree copies each system service's sources, the system seed, then each overlay folder
// (holding <repo>/ subfolders) under dst, and returns the service folders by name. The M2–M4
// evals keep the fixture without the seed; M5 applies it.
func SystemTree(dst string, overlays ...string) (map[string]string, error) {
	dirs := map[string]string{}
	layers := append([]string{filepath.Join(SystemDir(), "seed")}, overlays...)
	for _, name := range SystemServices {
		dir := filepath.Join(dst, name)
		if err := copyTree(filepath.Join(Sources(), name), dir); err != nil {
			return nil, err
		}
		for _, layer := range layers {
			if err := copyIfExists(filepath.Join(layer, name), dir); err != nil {
				return nil, err
			}
		}
		dirs[name] = dir
	}
	return dirs, nil
}

func copyIfExists(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return copyTree(src, dst)
}

// SeedSystem commits the system seed to main of each system service in the built fixture, as a
// teammate's earlier work, so Athel's cache clones and the developer's clones see it.
func SeedSystem(l *Layout) error { return builder{git: runGit}.seedSystem(l) }

func (b builder) seedSystem(l *Layout) error {
	for _, name := range SystemServices {
		r, ok := l.Repos[name]
		if !ok {
			return fmt.Errorf("the fixture has no %s repo", name)
		}
		work := filepath.Join(l.Root, "work", "system-"+name)
		if err := b.git(dev, l.Root, "clone", "-q", r.Bare, work); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(SystemDir(), "seed", name), work); err != nil {
			return err
		}
		for _, s := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Run " + name + " locally: addresses from the environment"}, {"push", "-q", "origin", "main"}} {
			if err := b.git(dev, work, s...); err != nil {
				return err
			}
		}
		if r.Clone != "" {
			if err := b.git(dev, r.Clone, "pull", "-q", "--ff-only", r.Bare, "main"); err != nil {
				return err
			}
		}
	}
	return nil
}
