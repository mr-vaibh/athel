package main

import (
	"errors"
	"os"
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
