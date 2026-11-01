package state

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

func TestLoadValidChange(t *testing.T) {
	s, _ := newTestStore(t)
	created := mustCreate(t, s, "Phone update")
	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "CHG-001" || got.State != Draft || got.Title != "Phone update" || got.CreatedAt != "2026-11-01T10:00:00.000Z" {
		t.Errorf("loaded change = %+v", got)
	}
	if got.PRD.Versions[0].SHA256 == "" || got.Budget.Status != BudgetOK {
		t.Errorf("prd/budget not initialised: %+v %+v", got.PRD, got.Budget)
	}
}

func TestLoadRejectsSchemaViolation(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	path := s.WS.ChangeFile(c.ID)
	orig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(string) string
		code aerr.Code
	}{
		{"unknown state", func(y string) string { return strings.Replace(y, "state: draft", "state: shipped", 1) }, aerr.StateSchema},
		{"extra field", func(y string) string { return y + "extra: 1\n" }, aerr.StateSchema},
		{"not yaml", func(string) string { return "{{{" }, aerr.StateSchema},
		{"empty file", func(string) string { return "" }, aerr.StateSchema},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tt.edit(string(orig))), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Get(c.ID); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Get("CHG-404"); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing change: err = %v", err)
	}
	if _, err := s.Get("nope"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("bad id: err = %v", err)
	}
	if _, err := Load(s.Reg, t.TempDir()); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("directory: err = %v", err)
	}
}

func TestSaveValidatesBeforeWrite(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	path := s.WS.ChangeFile(c.ID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c.State = "shipped"
	if err := Save(s.Reg, path, c); !aerr.HasCode(err, aerr.StateSchema) {
		t.Fatalf("err = %v, want E-STATE-SCHEMA", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("invalid change was written")
	}
}

func TestRoundTripStable(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "Round trip: quotes \" and colons: yes")
	note, step := "first: note", "investigate"
	c.Checkpoints = append(c.Checkpoints, Checkpoint{Name: "investigate.a", Step: &step, At: c.CreatedAt, Note: &note, Session: "s1", Trigger: "manual"})
	first, err := Encode(s.Reg, c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(s.Reg, first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(s.Reg, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("round trip changed the file:\n%s\n---\n%s", first, second)
	}
}

func TestDecodeRejectsTypeMismatchAfterSchema(t *testing.T) {
	// A file that passes the schema always decodes; this guards the KnownFields path directly.
	if _, err := Decode(schema.NewRegistry(), []byte("schema: athel/change@1\n")); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("err = %v", err)
	}
}

func FuzzLoadChange(f *testing.F) {
	s, _ := newTestStore(f)
	c := mustCreate(f, s, "seed")
	seed, err := Encode(s.Reg, c)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(""))
	f.Add([]byte("state: [unterminated"))
	f.Add([]byte("a: &x [*x, *x]"))
	reg := schema.NewRegistry()
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := Decode(reg, data)
		if err != nil {
			if _, ok := aerr.As(err); !ok {
				t.Fatalf("untyped error: %v", err)
			}
			return
		}
		if _, err := Encode(reg, got); err != nil {
			t.Fatalf("a change that decoded does not re-encode: %v", err)
		}
	})
}

// TestJSONKeysMatchYAMLKeys keeps --json output identical to change.yaml key names.
func TestJSONKeysMatchYAMLKeys(t *testing.T) {
	src, err := os.ReadFile("change.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("`yaml:\"([^\"]+)\"( json:\"([^\"]+)\")?`")
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if m[1] != m[3] {
			t.Errorf("yaml key %q has json key %q", m[1], m[3])
		}
	}
}
