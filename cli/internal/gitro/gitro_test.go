package gitro

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir := t.TempDir()
	if code, out, err := Run(dir, "init", "-q"); err != nil || code != 0 {
		t.Fatalf("init: %d %s %v", code, out, err)
	}
	if code, out, err := Run(dir, "rev-parse", "--is-inside-work-tree"); err != nil || code != 0 || strings.TrimSpace(out) != "true" {
		t.Errorf("rev-parse = %d %q %v", code, out, err)
	}
	if code, _, err := Run(filepath.Join(dir, "missing")); err != nil || code == 0 {
		t.Errorf("missing folder: exit %d %v", code, err)
	}
	t.Setenv("PATH", "")
	if _, _, err := Run(dir, "status"); err == nil {
		t.Error("git without a PATH started")
	}
}

func TestNetworkEnv(t *testing.T) {
	env := map[string]string{"PATH": "/bin", "HOME": "/h", "SSH_AUTH_SOCK": "/s", "AWS_SECRET_ACCESS_KEY": "x"}
	got := strings.Join(NetworkEnv(func(k string) string { return env[k] }), " ")
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "PATH=/bin", "HOME=/h", "SSH_AUTH_SOCK=/s"} {
		if !strings.Contains(got, want) {
			t.Errorf("env %q lacks %s", got, want)
		}
	}
	if strings.Contains(got, "AWS") || strings.Contains(got, "XDG") {
		t.Errorf("env %q carries more than git needs", got)
	}
}
