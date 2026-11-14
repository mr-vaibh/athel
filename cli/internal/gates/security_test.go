package gates

import (
	"strings"
	"testing"
)

func TestGosecLines(t *testing.T) {
	out := "main.go:1:1: G114: no timeouts (gosec)\napi/a.go:3:1: exported func (revive)\n2 issues:\n"
	if got := GosecLines(out); got != "main.go:1:1: G114: no timeouts (gosec)\n" {
		t.Errorf("%q", got)
	}
}

func TestDependencyChanged(t *testing.T) {
	if !DependencyChanged([]string{"src/a.go", "go.sum"}) || !DependencyChanged([]string{"web/package-lock.json"}) || DependencyChanged([]string{"src/go.go"}) {
		t.Error("dependency manifests and lockfiles, at any depth")
	}
}

func TestSecretsIn(t *testing.T) {
	got := SecretsIn(map[string]string{
		"src/a.go":   "token := \"ghp_0123456789abcdefghijABCDEFGHIJ012345\"\n",
		"src/b.go":   "x := 1\n",
		"config.env": "DB_PASSWORD=hunter2hunter2\n",
	})
	if len(got) != 2 || !strings.Contains(got[0], "config.env") || !strings.Contains(got[1], "src/a.go (github-token)") || strings.Contains(strings.Join(got, ""), "ghp_") {
		t.Errorf("by file and kind, never the value: %q", got)
	}
}

func TestVulnerabilities(t *testing.T) {
	out := "Scanning your code...\n\nVulnerability #1: GO-2026-0001\n    A bad bug in x/net\nVulnerability #2: GO-2026-0002\n"
	if got := Vulnerabilities(out); strings.Join(got, ";") != "Vulnerability #1: GO-2026-0001;Vulnerability #2: GO-2026-0002" {
		t.Errorf("%q", got)
	}
}

func TestSecurityGateDefault(t *testing.T) {
	var sec Gate
	for _, g := range Defaults(Go, nil) {
		if g.ID == "security-scan" {
			sec = g
		}
	}
	if sec.Waivable != SecurityOwner || sec.Stage != Full || sec.Pass != Security || strings.Join(sec.Command, " ") != "golangci-lint run --enable-only gosec ./..." {
		t.Errorf("%+v", sec)
	}
}
