package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mr-vaibh/athel/tests/fixtures/shop/fakes"
)

func TestMainRunsTheShim(t *testing.T) {
	origArgs, origExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = origArgs, origExit })
	got := -1
	exit = func(code int) { got = code }
	os.Args = []string{"/bin/kubectl", "config", "current-context"}
	main()
	if got != 0 {
		t.Errorf("exit = %d", got)
	}
	if code := run([]string{"/bin/praxis", "status"}, func(string) string { return t.TempDir() }, fakes.Load); code != 0 {
		t.Errorf("praxis status = %d", code)
	}
	if code := run([]string{"/bin/praxis", "status"}, os.Getenv, func() (*fakes.Data, error) { return nil, errors.New("no data") }); code != 1 {
		t.Errorf("load failure = %d", code)
	}
}

// TestStateFromGHConfigDir: Athel passes gh only its own settings, so the fake GitHub also finds
// its state folder through GH_CONFIG_DIR.
func TestStateFromGHConfigDir(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"GH_CONFIG_DIR": dir}
	argv := []string{"gh", "pr", "create", "--repo", "acme/users", "--head", "athel/x", "--base", "main", "--title", "feat: x", "--body", "b", "--draft"}
	if code := run(argv, func(k string) string { return env[k] }, fakes.Load); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "prs.json")); err != nil {
		t.Errorf("the PR is stored in GH_CONFIG_DIR: %v", err)
	}
}
