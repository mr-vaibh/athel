package systest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

func flows(t *testing.T) []Flow {
	t.Helper()
	f, err := LoadFlows(filepath.Join(shop.SystemDir(), "flows.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func reference(step string) string { return filepath.Join(shop.SystemDir(), "reference", step) }

func TestSystemCheckAcrossServices(t *testing.T) {
	dirs, err := shop.SystemTree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	results, err := Run(context.Background(), dirs, flows(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || len(Failed(results)) != 0 {
		t.Errorf("the seeded system passes every flow: %+v", results)
	}
}

// TestWrongOrderBreaksTheSystem proves the fixture can tell a safe merge order from an unsafe
// one: orders moving to users' v2 endpoint before users serves it breaks the customer flow.
// Without this, an eval that "merges in order" would prove nothing.
func TestWrongOrderBreaksTheSystem(t *testing.T) {
	tests := []struct {
		name   string
		merged []string
		broken string
	}{
		{"provider first", []string{"users-v2-provider"}, ""},
		{"provider then consumer", []string{"users-v2-provider", "users-v2-consumer"}, ""},
		{"consumer first", []string{"users-v2-consumer"}, "orders reads its customer from users: 502 users answered 404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var layers []string
			for _, m := range tt.merged {
				layers = append(layers, reference(m))
			}
			dirs, err := shop.SystemTree(t.TempDir(), layers...)
			if err != nil {
				t.Fatal(err)
			}
			results, err := Run(context.Background(), dirs, flows(t))
			if err != nil {
				t.Fatal(err)
			}
			failed := strings.Join(Failed(results), "; ")
			if tt.broken == "" && failed != "" || tt.broken != "" && !strings.HasPrefix(failed, tt.broken) {
				t.Errorf("failed flows %q, want %q", failed, tt.broken)
			}
		})
	}
}

func writeService(t *testing.T, main string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"go.mod": "module shop/broken\n\ngo 1.27\n", "main.go": main}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRunErrors(t *testing.T) {
	orig := startTimeout
	t.Cleanup(func() { startTimeout = orig })
	startTimeout = 300 * time.Millisecond
	tests := []struct {
		name  string
		dirs  map[string]string
		flows []Flow
		want  string
	}{
		{"build fails", map[string]string{"users": writeService(t, "package main\n\nfunc main() { undefined() }\n")}, nil, "build users"},
		{"never listens", map[string]string{"users": writeService(t, "package main\n\nfunc main() { println(\"bye\") }\n")}, nil, "users did not start"},
		{"flow on a stopped service", map[string]string{}, []Flow{{Name: "x", Service: "orders", Method: "GET", Path: "/"}}, "orders, which is not running"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Run(context.Background(), tt.dirs, tt.flows); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestFlowRequestError(t *testing.T) {
	s := &system{ports: map[string]int{"users": 1}}
	if _, err := s.run(context.Background(), Flow{Name: "bad", Service: "users", Method: "BAD METHOD", Path: "/"}); err == nil {
		t.Error("an invalid request is an error")
	}
	r, err := s.run(context.Background(), Flow{Name: "closed", Service: "users", Method: "GET", Path: "/"})
	if err != nil || r.OK || r.Got == "" {
		t.Errorf("a refused connection is a failed flow: %+v, %v", r, err)
	}
}

func TestLoadFlowsErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "flows.yaml")
	if err := os.WriteFile(bad, []byte("flows:\n  - {name: x, unknown: y}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFlows(bad); err == nil {
		t.Error("an unknown field is refused")
	}
	if _, err := LoadFlows(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("a missing file is an error")
	}
}
