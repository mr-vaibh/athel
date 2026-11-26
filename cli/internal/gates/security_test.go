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
	out := "=== Symbol Results ===\n\nVulnerability #1: GO-2026-6859\n    HTTP/1 server crash in net/http\n" +
		"  More info: https://pkg.go.dev/vuln/GO-2026-6859\n  Standard library\n    Found in: net/http@go1.27.3\n" +
		"    Fixed in: net/http@go1.27.4\n    Example traces found:\n      #1: main.go:15:31: users.main calls http.ListenAndServe\n\n" +
		"Vulnerability #2: GO-2026-0002\n  Module: example.com/evil\n    Found in: example.com/evil@v1.0.0\n    Fixed in: N/A\n" +
		"Vulnerability #3: GO-2026-0003\n"
	tests := []struct {
		id, text string
	}{
		{"GO-2026-6859", "GO-2026-6859 in net/http@go1.27.3 (the Go toolchain), fixed in net/http@go1.27.4"},
		{"GO-2026-0002", "GO-2026-0002 in example.com/evil@v1.0.0, with no fixed version"},
		{"GO-2026-0003", "GO-2026-0003"},
	}
	got := Vulnerabilities(out)
	if len(got) != len(tests) {
		t.Fatalf("%d findings, want %d: %v", len(got), len(tests), got)
	}
	for i, tt := range tests {
		if got[i].ID != tt.id || got[i].String() != tt.text {
			t.Errorf("#%d: %s %q, want %s %q", i+1, got[i].ID, got[i].String(), tt.id, tt.text)
		}
	}
}

func TestMayChangeDependencies(t *testing.T) {
	tests := []struct {
		paths []string
		want  bool
	}{
		{[]string{"go.mod"}, true},
		{[]string{"api/user.go", "phone/**"}, true},
		{[]string{"api/user.go", "api/user_test.go"}, false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := MayChangeDependencies(tt.paths); got != tt.want {
			t.Errorf("%v: %v, want %v", tt.paths, got, tt.want)
		}
	}
}

func TestSecurityGateDefault(t *testing.T) {
	var sec Gate
	for _, g := range Defaults(Go, nil) {
		if g.ID == "security-scan" {
			sec = g
		}
	}
	if sec.Waivable != SecurityOwner || sec.Stage != Full || sec.Pass != Security || strings.Join(sec.Command, " ") != "golangci-lint run --allow-parallel-runners --enable-only gosec ./..." {
		t.Errorf("%+v", sec)
	}
}
