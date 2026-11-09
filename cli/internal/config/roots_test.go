package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestSetUserRoots(t *testing.T) {
	reg := schema.NewRegistry()
	home := t.TempDir()
	if err := SetUserRoots(Paths{Home: home}, reg, []string{"~/Developer"}); err != nil {
		t.Fatal(err)
	}
	r, err := Load(Paths{Home: home}, reg)
	if err != nil || r.Effective.Origins["repos.search_roots"] != LayerUser {
		t.Fatalf("load = %+v %v", r, err)
	}
	user := filepath.Join(home, "user.yaml")
	write(t, user, "# my settings\nschema: athel/config@1 # old\nidentity: dev@example.com # me\nrepos: 3\n")
	if err := SetUserRoots(Paths{Home: home}, reg, []string{"~/Developer", "~/snabbit"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(user)
	for _, want := range []string{"# my settings", "schema: athel/config@2", "identity: dev@example.com # me", "search_roots: [~/Developer, ~/snabbit]"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("user.yaml lacks %q:\n%s", want, b)
		}
	}
}

func TestSetUserRootsTightenOnly(t *testing.T) {
	reg := schema.NewRegistry()
	home := t.TempDir()
	write(t, filepath.Join(home, "org", "athel.yaml"), "schema: athel/config@2\nrepos:\n  search_roots: [~/work]\n")
	user := filepath.Join(home, "user.yaml")
	write(t, user, "schema: athel/config@2\nidentity: dev\n")
	if err := SetUserRoots(Paths{Home: home}, reg, []string{"~/work", "~/Documents"}); !aerr.HasCode(err, aerr.PolicyConfigLoosens) {
		t.Fatalf("widening the org's roots: %v", err)
	}
	if b, _ := os.ReadFile(user); string(b) != "schema: athel/config@2\nidentity: dev\n" {
		t.Errorf("a rejected write was kept:\n%s", b)
	}
	fresh := t.TempDir()
	write(t, filepath.Join(fresh, "org", "athel.yaml"), "schema: athel/config@2\nrepos:\n  search_roots: [~/work]\n")
	if err := SetUserRoots(Paths{Home: fresh}, reg, []string{"~/Documents"}); !aerr.HasCode(err, aerr.PolicyConfigLoosens) {
		t.Fatalf("new user file widening the org's roots: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "user.yaml")); err == nil {
		t.Error("a rejected new user.yaml was kept")
	}
	if err := SetUserRoots(Paths{Home: fresh}, reg, []string{"~/work"}); err != nil {
		t.Errorf("the same roots as the org: %v", err)
	}
}

func TestSetUserRootsErrors(t *testing.T) {
	reg := schema.NewRegistry()
	tests := map[string]string{"not yaml": "a: [", "a list": "- a\n", "invalid after edit": "schema: athel/config@2\nbogus: 1\n"}
	for name, body := range tests {
		home := t.TempDir()
		write(t, filepath.Join(home, "user.yaml"), body)
		if err := SetUserRoots(Paths{Home: home}, reg, []string{"~/a"}); !aerr.HasCode(err, aerr.PolicyConfigInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := SetUserRoots(Paths{Home: t.TempDir()}, reg, []string{"relative"}); !aerr.HasCode(err, aerr.PolicyConfigInvalid) {
		t.Errorf("relative root: %v", err)
	}
	unreadable := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unreadable, "user.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SetUserRoots(Paths{Home: unreadable}, reg, []string{"~/a"}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable user layer: %v", err)
	}
	file := filepath.Join(t.TempDir(), "f")
	write(t, file, "")
	if err := SetUserRoots(Paths{Home: filepath.Join(file, "home")}, reg, []string{"~/a"}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("home under a file: %v", err)
	}
	ro := t.TempDir()
	if err := os.Chmod(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	if err := SetUserRoots(Paths{Home: ro}, reg, []string{"~/a"}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("read-only home: %v", err)
	}
	if err := SetUserRoots(Paths{Home: filepath.Join(ro, "new-home")}, reg, []string{"~/a"}); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("home that cannot be created: %v", err)
	}
}
