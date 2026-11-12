// Package shop builds the fixture company of doc 08 §Fixture system: five small services in two
// languages as real git repositories, with the traps M2's scenarios check. Everything is local:
// remotes are bare repositories, and commits have fixed authors and dates, so every build of the
// fixture has the same commit IDs.
package shop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Services are the fixture's repositories, in build order.
var Services = []string{"gateway", "notifications", "orders", "payments", "users"}

// NotCloned are repositories the developer has no local clone of; their code is reachable only
// through GitHub search or Athel's cache (trap: a consumer only in another repo).
var NotCloned = map[string]bool{"notifications": true}

// TeammateBranch is a teammate's work overlapping users/api (trap: active work by others).
const TeammateBranch = "teammate/profile-phone"

// Repo is one built repository.
type Repo struct {
	Remote string // normalised remote, github.com/acme/<name>
	Bare   string // the bare repository acting as GitHub
	Clone  string // the developer's clone, "" when not cloned
}

// Layout is a built fixture.
type Layout struct {
	Root  string
	Home  string // the developer's home folder: Developer/ holds the clones
	Repos map[string]Repo
}

const date = "2026-10-01T09:00:00Z"

func gitEnv(name, email, when string) []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email, "GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email,
		"GIT_AUTHOR_DATE=" + when, "GIT_COMMITTER_DATE=" + when, "LC_ALL=C"}
}

var dev = gitEnv("Shop Dev", "dev@shop.example", date)

// runGit runs git with exactly env, so the fixture never depends on this machine's git config.
func runGit(env []string, dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // fixed git subcommands on the fixture's own folders
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Sources is the folder holding each service's source tree.
func Sources() string {
	_, file, _, _ := runtime.Caller(0) // this file's own path is always known
	return filepath.Join(filepath.Dir(file), "testdata", "repos")
}

// Build creates the fixture under root.
func Build(root string) (*Layout, error) { return builder{git: runGit}.build(root) }

// builder carries the git runner, so tests can make a chosen git step fail.
type builder struct {
	git func(env []string, dir string, args ...string) error
}

func (b builder) build(root string) (*Layout, error) {
	l := &Layout{Root: root, Home: filepath.Join(root, "home"), Repos: map[string]Repo{}}
	for _, name := range Services {
		r, err := b.buildRepo(root, l.Home, name)
		if err != nil {
			return nil, err
		}
		l.Repos[name] = r
	}
	return l, nil
}

func (b builder) buildRepo(root, home, name string) (Repo, error) {
	work := filepath.Join(root, "work", name)
	if err := copyTree(filepath.Join(Sources(), name), work); err != nil {
		return Repo{}, err
	}
	r := Repo{Remote: "github.com/acme/" + name, Bare: filepath.Join(root, "remotes", "acme", name+".git")}
	steps := [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-q", "-m", "Initial " + name}}
	for _, s := range steps {
		if err := b.git(dev, work, s...); err != nil {
			return Repo{}, err
		}
	}
	if name == "users" {
		if err := b.teammateWork(work); err != nil {
			return Repo{}, err
		}
	}
	if err := b.preparedBranches(work, name); err != nil {
		return Repo{}, err
	}
	if err := b.git(dev, root, "clone", "-q", "--bare", work, r.Bare); err != nil {
		return Repo{}, err
	}
	if NotCloned[name] {
		return r, nil
	}
	r.Clone = filepath.Join(home, "Developer", name)
	if err := b.git(dev, root, "clone", "-q", r.Bare, r.Clone); err != nil {
		return Repo{}, err
	}
	return r, b.git(dev, r.Clone, "remote", "set-url", "origin", "git@github.com:acme/"+name+".git")
}

// teammateWork leaves a teammate's branch that changes users/api, where a phone change would land.
func (b builder) teammateWork(work string) error {
	mate := gitEnv("Teammate", "teammate@shop.example", "2026-10-28T15:00:00Z")
	if err := b.git(mate, work, "switch", "-q", "-c", TeammateBranch); err != nil {
		return err
	}
	f := filepath.Join(work, "api", "user.go")
	src, err := os.ReadFile(f) //nolint:gosec // a fixture file this function just copied
	if err != nil {
		return err
	}
	if err := os.WriteFile(f, append(src, []byte("\n// Phone formats are being normalised to E.164 (teammate, in progress).\n")...), 0o600); err != nil { //nolint:gosec // a file inside the fixture's own work tree
		return err
	}
	for _, s := range [][]string{{"commit", "-q", "-am", "Normalise phone formats"}, {"switch", "-q", "main"}} {
		if err := b.git(mate, work, s...); err != nil {
			return err
		}
	}
	return nil
}

// branchesDir holds prepared branches per repo: testdata/branches/<repo>/<branch>/ is copied
// over main and committed as <branch> ("__" stands for "/"). M3's actual blast radius tests
// diff them against main (eval/phone-planned matches eval PRD 1's plan; eval/phone-extra also
// renames the users.updated event, an impact the plan does not have).
func branchesDir() string { return filepath.Join(filepath.Dir(Sources()), "branches") }

// preparedBranches commits each prepared branch of repo name from main.
func (b builder) preparedBranches(work, name string) error {
	dirs, err := os.ReadDir(filepath.Join(branchesDir(), name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	author := gitEnv("Athel eval", "eval@shop.example", "2026-11-05T10:00:00Z")
	for _, d := range dirs {
		branch := strings.ReplaceAll(d.Name(), "__", "/")
		if err := b.git(author, work, "switch", "-q", "-c", branch, "main"); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(branchesDir(), name, d.Name()), work); err != nil {
			return err
		}
		for _, s := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Prepared branch " + branch}, {"switch", "-q", "main"}} {
			if err := b.git(author, work, s...); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p) // p is always inside src
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		b, err := os.ReadFile(p) //nolint:gosec // fixture sources inside the repository
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600) //nolint:gosec // target is inside the destination the caller chose
	})
}
