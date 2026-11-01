package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestIntegrityAcceptCommand(t *testing.T) {
	h := setup(t)
	if v := h.mustOK("state", "show", "CHG-001"); v["integrity"] != "ok" {
		t.Errorf("fresh change: %v", v["integrity"])
	}
	cf := filepath.Join(h.root, ".athel", "changes", "CHG-001", "change.yaml")
	b, err := os.ReadFile(cf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf, []byte(strings.Replace(string(b), "title: Phone update", "title: Phone update v2", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if v := h.mustOK("state", "show", "CHG-001"); v["integrity"] != "edited" {
		t.Errorf("edited change: %v", v["integrity"])
	}
	code, env := h.runJSON("state", "transition", "CHG-001", "--to", "investigating", "--by", "athel", "--reason", "r")
	if code != aerr.ExitIntegrity || errCode(env) != string(aerr.IntegrityState) {
		t.Errorf("frozen change: exit %d env %v", code, env)
	}
	for _, args := range [][]string{{"CHG-001"}, {"CHG-001", "--reason", " "}, {"--reason", "x"}} {
		if code, _ := h.runJSON(append([]string{"integrity", "accept"}, args...)...); code != aerr.ExitUsage {
			t.Errorf("usage %v: exit %d", args, code)
		}
	}
	h.env["ATHEL_SESSION_ID"] = ""
	if v := h.mustOK("integrity", "accept", "CHG-001", "--reason", "renamed by the developer"); v["integrity"] != "ok" || v["title"] != "Phone update v2" {
		t.Errorf("accept = %v", v)
	}
	if code, _ := h.runJSON("integrity", "accept", "CHG-001", "--reason", "again"); code != aerr.ExitState {
		t.Errorf("nothing to accept: exit %d", code)
	}
	if code, _ := newHarness(t).runJSON("integrity", "accept", "CHG-001", "--reason", "x"); code != aerr.ExitState {
		t.Errorf("no workspace: exit %d", code)
	}
}
