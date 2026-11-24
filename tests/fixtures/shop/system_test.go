package shop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedSystem(t *testing.T) {
	l, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedSystem(l); err != nil {
		t.Fatal(err)
	}
	for _, name := range SystemServices {
		r := l.Repos[name]
		out, err := exec.Command("git", "-C", r.Bare, "show", "main:main_test.go").CombinedOutput()
		if err != nil {
			t.Errorf("%s: main lacks the seed: %v %s", name, err, out)
		}
		if b, err := os.ReadFile(filepath.Join(r.Clone, "main.go")); err != nil || !strings.Contains(string(b), "ADDR") {
			t.Errorf("%s: the developer's clone is not up to date: %v", name, err)
		}
	}
}

func TestSystemTree(t *testing.T) {
	dirs, err := SystemTree(t.TempDir(), filepath.Join(SystemDir(), "reference", "users-v2-provider"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dirs["users"], "api", "user.go"))
	if err != nil || !strings.Contains(string(b), "/v2/users/") {
		t.Errorf("the overlay is applied over the seed: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dirs["orders"], "api", "customer.go")); err != nil || len(b) == 0 {
		t.Errorf("the seed is applied: %v", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SystemTree(filepath.Join(file, "dst")); err == nil {
		t.Error("a tree under a file is an error")
	}
	overlay := t.TempDir()
	if err := os.WriteFile(filepath.Join(overlay, "users"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SystemTree(t.TempDir(), overlay); err == nil {
		t.Error("an overlay entry that is not a folder is an error")
	}
}

func TestSeedSystemFailures(t *testing.T) {
	l, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"clone", "add", "commit", "push", "pull"} {
		t.Run(step, func(t *testing.T) {
			b := builder{git: func(env []string, dir string, args ...string) error {
				if args[0] == step {
					return os.ErrPermission
				}
				return runGit(env, dir, args...)
			}}
			copied := *l
			copied.Root = t.TempDir()
			if err := b.seedSystem(&copied); err == nil {
				t.Errorf("a failing git %s was not reported", step)
			}
		})
	}
	if err := SeedSystem(&Layout{Root: t.TempDir(), Repos: map[string]Repo{}}); err == nil || !strings.Contains(err.Error(), "no users repo") {
		t.Errorf("a missing repo: %v", err)
	}
	fresh, err := Build(t.TempDir()) // the subtests above pushed the seed to l's remotes
	if err != nil {
		t.Fatal(err)
	}
	notCloned := *fresh
	notCloned.Repos = map[string]Repo{}
	for name, r := range fresh.Repos {
		r.Clone = ""
		notCloned.Repos[name] = r
	}
	if err := SeedSystem(&notCloned); err != nil {
		t.Errorf("repos without a developer clone are seeded too: %v", err)
	}
	copyFails := *l
	copyFails.Root = t.TempDir()
	b := builder{git: func(env []string, dir string, args ...string) error {
		if args[0] == "clone" {
			return os.WriteFile(args[len(args)-1], nil, 0o600) // a file where the work folder should be
		}
		return runGit(env, dir, args...)
	}}
	if err := os.MkdirAll(filepath.Join(copyFails.Root, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.seedSystem(&copyFails); err == nil {
		t.Error("a failing copy of the seed was not reported")
	}
}
