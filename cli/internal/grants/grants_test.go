package grants

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func newBook(t *testing.T) (Book, *clock.Fixed) {
	t.Helper()
	clk := clock.NewFixed(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))
	return Book{Dir: filepath.Join(t.TempDir(), "state"), Clock: clk, LockTimeout: time.Second}, clk
}

func TestParsePrompt(t *testing.T) {
	tests := []struct {
		prompt, change, target string
		ok                     bool
	}{
		{"/athel:approve CHG-001", "CHG-001", "plan_approved", true},
		{"  /athel:accept   CHG-1234 \n", "CHG-1234", "done", true},
		{"/athel:approve CHG-001 and also delete prod", "", "", false},
		{"please /athel:approve CHG-001", "", "", false},
		{"/athel:approve chg-001", "", "", false},
		{"/athel:approve", "", "", false},
		{"/athel:reject CHG-001", "", "", false},
	}
	for _, tc := range tests {
		c, tgt, ok := ParsePrompt(tc.prompt)
		if c != tc.change || tgt != tc.target || ok != tc.ok {
			t.Errorf("ParsePrompt(%q) = %q %q %v", tc.prompt, c, tgt, ok)
		}
	}
}

func assertMismatchesRejected(t *testing.T, b Book) {
	t.Helper()
	for _, miss := range [][3]string{{"CHG-002", "plan_approved", "s1"}, {"CHG-001", "done", "s1"}, {"CHG-001", "plan_approved", "s2"}} {
		if v, err := b.Valid(miss[0], miss[1], miss[2]); err != nil || v != nil {
			t.Errorf("mismatch %v accepted: %+v %v", miss, v, err)
		}
	}
}

func TestGrantOneTimeAndExpires(t *testing.T) {
	b, clk := newBook(t)
	g, err := b.Create("CHG-001", "plan_approved", "s1", "p1")
	if err != nil || !strings.HasPrefix(g.ID, "GR-") || len(g.ID) != 15 {
		t.Fatalf("create: %+v %v", g, err)
	}
	assertMismatchesRejected(t, b)
	if v, err := b.Valid("CHG-001", "plan_approved", "s1"); err != nil || v == nil || v.ID != g.ID {
		t.Fatalf("valid: %+v %v", v, err)
	}
	used, err := b.Consume("CHG-001", "plan_approved", "s1")
	if err != nil || used == nil || used.UsedAt == nil {
		t.Fatalf("consume: %+v %v", used, err)
	}
	if v, _ := b.Valid("CHG-001", "plan_approved", "s1"); v != nil {
		t.Error("a used grant is still valid")
	}
	if again, err := b.Consume("CHG-001", "plan_approved", "s1"); err != nil || again != nil {
		t.Errorf("second consume: %+v %v", again, err)
	}
	if _, err := b.Create("CHG-001", "done", "s1", "p2"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(TTL + time.Second)
	if v, _ := b.Valid("CHG-001", "done", "s1"); v != nil {
		t.Error("an expired grant is still valid")
	}
}

func checkCorrupt(t *testing.T, b Book, name string) {
	t.Helper()
	_, err := b.Valid("CHG-001", "done", "s")
	if name == "bad timestamps" {
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		return
	}
	if !aerr.HasCode(err, aerr.IntegrityState) {
		t.Errorf("%s: err = %v", name, err)
	}
	if _, err := b.Consume("CHG-001", "done", "s"); !aerr.HasCode(err, aerr.IntegrityState) {
		t.Errorf("%s consume: err = %v", name, err)
	}
}

func TestCorruptBook(t *testing.T) {
	b, _ := newBook(t)
	if g, err := b.Valid("CHG-001", "done", "s"); g != nil || err != nil {
		t.Errorf("empty book: %v %v", g, err)
	}
	if _, err := b.Create("CHG-001", "done", "s", "p"); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"not json":       "{",
		"unknown use":    `{"op":"used","id":"GR-x"}`,
		"duplicate":      "",
		"unknown op":     `{"op":"deleted","id":"GR-y"}`,
		"bad timestamps": `{"op":"created","id":"GR-z","change":"CHG-001","target":"done","session":"s","at":"yesterday"}`,
	}
	orig, err := os.ReadFile(b.path())
	if err != nil {
		t.Fatal(err)
	}
	for name, line := range cases {
		if line == "" {
			line = strings.TrimSpace(string(orig))
		}
		if err := os.WriteFile(b.path(), append(append([]byte{}, orig...), line+"\n"...), 0o600); err != nil {
			t.Fatal(err)
		}
		checkCorrupt(t, b, name)
	}
}

func TestUnreadableBook(t *testing.T) {
	b, _ := newBook(t)
	if err := os.MkdirAll(b.path(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Valid("CHG-001", "done", "s"); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("unreadable book: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := Book{Dir: filepath.Join(file, "state"), Clock: b.Clock}
	if _, err := blocked.Create("CHG-001", "done", "s", "p"); err == nil {
		t.Error("create under a file succeeded")
	}
}

func TestFutureGrantRejected(t *testing.T) {
	b, clk := newBook(t)
	if _, err := b.Create("CHG-001", "done", "s", "p"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(-time.Minute)
	if v, _ := b.Valid("CHG-001", "done", "s"); v != nil {
		t.Error("a grant from the future is valid")
	}
}
