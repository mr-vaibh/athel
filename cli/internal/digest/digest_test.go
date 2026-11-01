package digest

import (
	"os"
	"path/filepath"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// sha256("abc") from FIPS 180-2 test vectors.
const abc = "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

func TestBytes(t *testing.T) {
	if got := Bytes([]byte("abc")); got != abc {
		t.Errorf("Bytes = %s", got)
	}
}

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prd.md")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := File(path)
	if err != nil || got != abc {
		t.Errorf("File = %s, %v", got, err)
	}
	if _, err := File(filepath.Join(t.TempDir(), "missing")); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("missing file: err = %v", err)
	}
	if _, err := File(t.TempDir()); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("directory: err = %v", err)
	}
}

func TestCanonicalJSONOrderIndependent(t *testing.T) {
	a := map[string]any{"b": 1, "a": map[string]any{"y": true, "x": []any{"1", 2}}}
	b := map[string]any{"a": map[string]any{"x": []any{"1", 2}, "y": true}, "b": 1}
	ha, err := CanonicalJSON(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := CanonicalJSON(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("hashes differ: %s vs %s", ha, hb)
	}
	hc, err := CanonicalJSON(map[string]any{"b": 2})
	if err != nil || hc == ha {
		t.Errorf("different value gave same hash or error: %v", err)
	}
	if _, err := CanonicalJSON(func() {}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("unencodable value: err = %v", err)
	}
}
