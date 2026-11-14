package shop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = dev
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}

func head(t *testing.T, dir, ref string) string { return gitOut(t, dir, "rev-parse", ref) }

func TestGeneratorDeterministic(t *testing.T) {
	t.Parallel()
	a, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Services {
		if head(t, a.Repos[name].Bare, "main") != head(t, b.Repos[name].Bare, "main") {
			t.Errorf("%s: commit IDs differ between builds", name)
		}
	}
	if head(t, a.Repos["users"].Bare, TeammateBranch) != head(t, b.Repos["users"].Bare, TeammateBranch) {
		t.Error("teammate branch differs between builds")
	}
}

func TestLayout(t *testing.T) {
	t.Parallel()
	l, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range Services {
		r := l.Repos[name]
		if r.Remote != "github.com/acme/"+name {
			t.Errorf("%s remote = %s", name, r.Remote)
		}
		if NotCloned[name] != (r.Clone == "") {
			t.Errorf("%s clone = %q", name, r.Clone)
		}
		if r.Clone != "" {
			if out := gitOut(t, r.Clone, "remote", "get-url", "origin"); out != "git@github.com:acme/"+name+".git" {
				t.Errorf("%s origin = %q", name, out)
			}
		}
	}
}

// read returns a fixture source file.
func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(Sources(), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTrapsPresent(t *testing.T) {
	traps := []struct{ name, file, want string }{
		{"alias: cluster name differs from code", "users/deploy/k8s.yaml", "name: user-svc"},
		{"alias: image names the repo", "users/deploy/k8s.yaml", "image: ghcr.io/acme/users"},
		{"dead endpoint still called", "orders/clients/users.go", "/legacy-profile"},
		{"consumer only in another repo", "notifications/src/consumers/userUpdated.ts", `"users.updated.v1"`},
		{"phone owned by users", "users/migrations/001_users.sql", "phone TEXT"},
		{"phone duplicated in notifications", "notifications/migrations/001_contacts.sql", "phone   TEXT"},
		{"gateway hop: orders calls the gateway", "orders/clients/payments.go", "/payments/charge"},
		{"gateway hop: route to payments", "gateway/routes.yaml", "service: payments"},
	}
	for _, tt := range traps {
		if !strings.Contains(read(t, tt.file), tt.want) {
			t.Errorf("trap %q: %s lacks %q", tt.name, tt.file, tt.want)
		}
	}
	if strings.Contains(read(t, "users/api/user.go"), "legacy-profile") {
		t.Error("users must no longer serve the legacy profile endpoint")
	}
}

// TestGoServicesBuildAndTest: M4's gates run each Go service's own build, format check, vet and
// tests, so on main every one of them must pass, and every library package must have tests
// (a service's main package only starts the server).
func TestGoServicesBuildAndTest(t *testing.T) {
	t.Parallel()
	l, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"users", "orders", "gateway"} {
		dir := filepath.Join(l.Root, "work", name)
		for _, args := range [][]string{{"go", "vet", "./..."}, {"go", "test", "-count=1", "./..."}, {"gofmt", "-l", "."}} {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOWORK=off")
			out, err := cmd.CombinedOutput()
			if err != nil || (args[0] == "gofmt" && len(out) > 0) {
				t.Errorf("%s: %v failed: %v %s", name, args, err, out)
			}
			if args[1] == "test" {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.Contains(line, "[no test files]") && !strings.Contains(line, "shop/"+name+"\t") {
						t.Errorf("%s: a library package has no tests: %s", name, line)
					}
				}
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".golangci.yml")); err != nil {
			t.Errorf("%s has no lint config: %v", name, err)
		}
	}
}

func TestBuildErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(filepath.Join(file, "root")); err == nil {
		t.Error("build under a file succeeded")
	}
	if err := runGit(dev, t.TempDir(), "not-a-command"); err == nil || !strings.Contains(err.Error(), "not-a-command") {
		t.Errorf("git error = %v", err)
	}
	if err := (builder{git: runGit}).teammateWork(t.TempDir()); err == nil {
		t.Error("teammate work outside a repo succeeded")
	}
}

func TestEveryGitStepFailureReported(t *testing.T) {
	t.Parallel()
	steps := []string{"init", "add", "commit", "switch", "clone", "remote"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			b := builder{git: func(env []string, dir string, args ...string) error {
				if args[0] == step {
					return os.ErrPermission
				}
				return runGit(env, dir, args...)
			}}
			if _, err := b.build(t.TempDir()); err == nil {
				t.Errorf("a failing git %s was not reported", step)
			}
		})
	}
	back := builder{git: func(env []string, dir string, args ...string) error {
		if len(args) == 3 && args[0] == "switch" && args[2] == "main" {
			return os.ErrPermission
		}
		return runGit(env, dir, args...)
	}}
	if _, err := back.build(t.TempDir()); err == nil {
		t.Error("a failing switch back to main was not reported")
	}
}

func TestCopyTreeErrors(t *testing.T) {
	if err := copyTree(filepath.Join(t.TempDir(), "none"), t.TempDir()); err == nil {
		t.Error("copy of a missing tree succeeded")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "secret"), []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, t.TempDir()); err == nil {
		t.Error("copy of an unreadable file succeeded")
	}
	ok := t.TempDir()
	if err := os.WriteFile(filepath.Join(ok, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(ok, filepath.Join(file, "dst")); err == nil {
		t.Error("copy under a file succeeded")
	}
	work := t.TempDir()
	if err := runGit(dev, work, "init", "-q", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	if err := (builder{git: runGit}).teammateWork(work); err == nil {
		t.Error("teammate work without api/user.go succeeded")
	}
}

// TestPreparedBranches: the actual blast radius tests diff these against main (M3 task 10).
func TestPreparedBranches(t *testing.T) {
	t.Parallel()
	a, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare := a.Repos["users"].Bare
	for branch, want := range map[string]string{
		"eval/phone-planned": "api/user.go migrations/002_phone_number.sql",
		"eval/phone-extra":   "api/user.go events/publish.go migrations/002_phone_number.sql",
	} {
		if got := strings.Join(strings.Fields(gitOut(t, bare, "diff", "--name-only", "main", branch)), " "); got != want {
			t.Errorf("%s changes %q, want %q", branch, got, want)
		}
	}
	b, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if head(t, bare, "eval/phone-extra") != head(t, b.Repos["users"].Bare, "eval/phone-extra") {
		t.Error("prepared branches differ between builds")
	}
}

func TestPreparedBranchFailures(t *testing.T) {
	t.Parallel()
	for name, failing := range map[string]func(args []string) bool{
		"create": func(args []string) bool {
			return len(args) > 3 && args[0] == "switch" && args[2] == "-c" && strings.HasPrefix(args[3], "eval/")
		},
		"commit": func(args []string) bool {
			return args[0] == "commit" && strings.Contains(strings.Join(args, " "), "Prepared branch")
		},
	} {
		b := builder{git: func(env []string, dir string, args ...string) error {
			if failing(args) {
				return os.ErrPermission
			}
			return runGit(env, dir, args...)
		}}
		if _, err := b.build(t.TempDir()); err == nil {
			t.Errorf("%s: a failing prepared branch was not reported", name)
		}
	}
}
