package athel

import (
	"encoding/json"
	"io/fs"
	"testing"
)

func TestEmbeddedSchemasAreJSON(t *testing.T) {
	names, err := fs.Glob(Schemas, "schemas/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) < 3 {
		t.Fatalf("embedded schemas = %v, want at least change, config, hook-event", names)
	}
	for _, name := range names {
		b, err := fs.ReadFile(Schemas, name)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) {
			t.Errorf("%s is not valid JSON", name)
		}
	}
}
