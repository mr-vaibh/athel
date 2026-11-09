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
