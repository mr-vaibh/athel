package cli

import (
	"os"
	"path/filepath"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestPolicyCommands(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	v := h.mustOK("policy", "validate")
	if v["valid"] != true || len(v["layers"].(map[string]any)) != 3 {
		t.Errorf("validate = %v", v)
	}
	show := h.mustOK("policy", "show")
	if show["default"] != "prod" || show["tools"].(map[string]any)["raptor"] == nil {
		t.Errorf("show = %v", show)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "tools.yaml"), []byte("schema: athel/tools@1\ntools:\n  - {id: kubectl, kind: cli, match: {command: kubectl}, read: [get, exec]}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"policy", "validate"}, {"policy", "show"}} {
		if code, env := h.runJSON(args...); code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyConfigLoosens) {
			t.Errorf("%v: exit %d env %v", args, code, env)
		}
	}
	if code, _ := h.runJSON("policy", "validate", "x"); code != aerr.ExitUsage {
		t.Errorf("extra arg: exit %d", code)
	}
	if code, _ := h.runJSON("policy", "show", "x"); code != aerr.ExitUsage {
		t.Errorf("extra arg: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("policy", "validate"); code != aerr.ExitPolicy {
		t.Errorf("broken athel.yaml: exit %d", code)
	}
	nohome := newHarness(t)
	nohome.home = ""
	if code, _ := nohome.runJSON("policy", "validate"); code != aerr.ExitState {
		t.Errorf("no home: exit %d", code)
	}
}
