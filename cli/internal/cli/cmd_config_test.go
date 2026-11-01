package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestConfigValidateExitCodes(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	v := h.mustOK("config", "validate")
	if v["valid"] != true || !strings.HasPrefix(v["effective_hash"].(string), "sha256:") || len(v["layers"].([]any)) != 2 {
		t.Errorf("validate = %v", v)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "athel.yaml"), []byte("schema: athel/config@1\nlocks:\n  ttl_min: 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, env := h.runJSON("config", "validate")
	if code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyConfigLoosens) {
		t.Errorf("loosening: exit %d env %v", code, env)
	}
	if err := os.WriteFile(filepath.Join(h.root, ".athel", "config", "athel.yaml"), []byte("schema: athel/config@1\nbogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, env = h.runJSON("config", "validate")
	if code != aerr.ExitPolicy || errCode(env) != string(aerr.PolicyConfigInvalid) {
		t.Errorf("schema violation: exit %d env %v", code, env)
	}
	outside := newHarness(t)
	if v := outside.mustOK("config", "validate"); len(v["layers"].([]any)) != 1 {
		t.Errorf("outside a workspace only the built-in layer applies: %v", v)
	}
}

func TestConfigShowOrigins(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("schema: athel/config@1\nidentity: dev@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	eff := h.mustOK("config", "show", "--effective")
	origins := eff["origins"].(map[string]any)
	if origins["identity"] != "user" || origins["budgets.default.max_cost_usd"] != "builtin" || eff["effective_hash"] == "" {
		t.Errorf("effective = %v", eff)
	}
	user := h.mustOK("config", "show", "--layer", "user")
	if user["values"].(map[string]any)["identity"] != "dev@example.com" || user["layer"].(map[string]any)["name"] != "user" {
		t.Errorf("user layer = %v", user)
	}
	if code, env := h.runJSON("config", "show", "--layer", "org"); code != aerr.ExitState || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("absent layer: exit %d env %v", code, env)
	}
	if code, _ := h.runJSON("config", "show", "--layer", "team"); code != aerr.ExitUsage {
		t.Errorf("unknown layer: exit %d", code)
	}
	if err := os.WriteFile(filepath.Join(h.home, "user.yaml"), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.runJSON("config", "show"); code != aerr.ExitPolicy {
		t.Errorf("broken layer: exit %d", code)
	}
	if code, _ := h.runJSON("config", "validate", "extra"); code != aerr.ExitUsage {
		t.Errorf("extra arg: exit %d", code)
	}
}

func TestSchemaValidateInfersSchema(t *testing.T) {
	h := newHarness(t)
	h.mustOK("init")
	h.mustOK("change", "new", "--title", "t", "--prd-text", "p")
	rel := filepath.Join(".athel", "changes", "CHG-001", "change.yaml")
	if v := h.mustOK("schema", "validate", rel); v["schema"] != "athel/change@2" || v["valid"] != true {
		t.Errorf("inferred = %v", v)
	}
	abs := filepath.Join(h.root, ".athel", "config", "athel.yaml")
	if v := h.mustOK("schema", "validate", abs, "--schema", "athel/config@1"); v["valid"] != true {
		t.Errorf("explicit = %v", v)
	}
	bad := filepath.Join(h.root, "bad.yaml")
	cases := map[string]string{"no schema field": "a: 1\n", "violation": "schema: athel/config@1\nbogus: 1\n", "not yaml": "a: ["}
	for name, content := range cases {
		if err := os.WriteFile(bad, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, env := h.runJSON("schema", "validate", bad); code != aerr.ExitState || errCode(env) != string(aerr.StateSchema) {
			t.Errorf("%s: exit %d env %v", name, code, env)
		}
	}
	if code, env := h.runJSON("schema", "validate", "missing.yaml"); code != aerr.ExitState || errCode(env) != string(aerr.StateNotFound) {
		t.Errorf("missing file: exit %d env %v", code, env)
	}
	if code, env := h.runJSON("schema", "validate", h.root); code != aerr.ExitState || errCode(env) != string(aerr.StateIO) {
		t.Errorf("directory: exit %d env %v", code, env)
	}
	if code, _ := h.runJSON("schema", "validate"); code != aerr.ExitUsage {
		t.Errorf("no file: exit %d", code)
	}
}
