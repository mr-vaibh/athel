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

func TestVersionOf(t *testing.T) {
	if got, err := versionOf("echo")(); err != nil || got != "--version" {
		t.Errorf("echo probe = %q, %v", got, err)
	}
	if _, err := versionOf("athel-no-such-binary")(); err == nil {
		t.Error("missing binary did not fail")
	}
}
