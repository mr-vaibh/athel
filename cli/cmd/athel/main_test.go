package main

import (
	"errors"
	"os"
	"testing"
)

func TestMainExitsWithRunCode(t *testing.T) {
	origArgs, origExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = origArgs, origExit })
	os.Args = []string{"athel", "no-such-command"}
	got := -1
	exit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main exit code = %d, want 2 (usage)", got)
	}
}

func TestEnvironment(t *testing.T) {
	env := environment(os.Getwd)
	if env.Cwd == "" || env.Version != version || env.Clock == nil || env.Getenv == nil || env.UserHome == nil {
		t.Errorf("environment = %+v", env)
	}
	broken := environment(func() (string, error) { return "", errors.New("gone") })
	if broken.Cwd != "." {
		t.Errorf("cwd fallback = %q", broken.Cwd)
	}
}
