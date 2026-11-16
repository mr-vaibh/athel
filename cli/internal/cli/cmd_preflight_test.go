package cli

import (
	"strings"
	"testing"
)

// TestGHKeepsItsOwnSettings: gh reaches GitHub with the developer's token login or config folder,
// and nothing else of the environment leaks to it.
func TestGHKeepsItsOwnSettings(t *testing.T) {
	vars := map[string]string{"PATH": "/bin", "HOME": "/h", "GH_TOKEN": "tok", "GH_CONFIG_DIR": "/cfg", "GH_HOST": "ghe.example", "AWS_SECRET_ACCESS_KEY": "no"}
	env := strings.Join(ghEnv(func(k string) string { return vars[k] }), " ")
	for _, want := range []string{"GH_TOKEN=tok", "GH_CONFIG_DIR=/cfg", "GH_HOST=ghe.example", "HOME=/h", "PATH=/bin"} {
		if !strings.Contains(env, want) {
			t.Errorf("gh lacks %s: %s", want, env)
		}
	}
	if strings.Contains(env, "AWS_") {
		t.Errorf("gh gets nothing else: %s", env)
	}
}
