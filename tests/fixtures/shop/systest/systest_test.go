package systest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

func reference(step string) string { return filepath.Join(shop.SystemDir(), "reference", step) }

func TestSystemCheckAcrossServices(t *testing.T) {
	dirs, err := shop.SystemTree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	results, err := Run(context.Background(), dirs)
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
			results, err := Run(context.Background(), dirs)
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

// TestCheckCannotRun: a service that does not build is an error, not a failed flow.
func TestCheckCannotRun(t *testing.T) {
	dirs, err := shop.SystemTree(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirs["users"], "broken.go"), []byte("package main\n\nfunc broken() { undefined() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), dirs); err == nil || !strings.Contains(err.Error(), "build users") {
		t.Errorf("err = %v", err)
	}
}
