package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

func TestRun(t *testing.T) {
	dirs, err := shop.SystemTree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	broken, err := shop.SystemTree(t.TempDir(), filepath.Join(shop.SystemDir(), "reference", "users-v2-consumer"))
	if err != nil {
		t.Fatal(err)
	}
	flows := filepath.Join(shop.SystemDir(), "flows.yaml")
	args := func(d map[string]string) []string {
		return []string{"--flows", flows, "--users", d["users"], "--orders", d["orders"], "--gateway", d["gateway"]}
	}
	tests := []struct {
		name    string
		argv    []string
		code    int
		out     string
		errText string
	}{
		{"every flow passes", args(dirs), 0, `"ok":true`, ""},
		{"a flow fails", args(broken), 1, `"ok":false`, ""},
		{"no flows file", []string{"--users", dirs["users"]}, 2, "", "--flows is required"},
		{"unreadable flows", []string{"--flows", filepath.Join(t.TempDir(), "none.yaml")}, 2, "", "no such file"},
		{"bad flag", []string{"--nope"}, 2, "", "flag provided but not defined"},
		{"check cannot run", []string{"--flows", flows}, 2, "", "not running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut strings.Builder
			if code := run(tt.argv, &out, &errOut); code != tt.code || !strings.Contains(out.String(), tt.out) || !strings.Contains(errOut.String(), tt.errText) {
				t.Errorf("code %d out %q err %q; want %d %q %q", code, out.String(), errOut.String(), tt.code, tt.out, tt.errText)
			}
		})
	}
}

func TestEntryPoint(t *testing.T) {
	origArgs, origExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = origArgs, origExit })
	got := -1
	exit = func(code int) { got = code }
	os.Args = []string{"shopsys"}
	main()
	if got != 2 {
		t.Errorf("exit = %d", got)
	}
}
