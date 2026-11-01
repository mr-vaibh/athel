package state

import (
	"bytes"
	"errors"
	"io/fs"
	"os"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// Decode validates data against its change schema and decodes it. Validation runs on the
// generic document first, so a file that does not match its schema never reaches the typed
// model. A change@1 file is validated as @1 and upgraded in memory to @2.
func Decode(reg *schema.Registry, data []byte) (*Change, error) {
	doc, err := schema.DecodeYAML(data)
	if err != nil {
		return nil, err
	}
	id := SchemaID
	if m, ok := doc.(map[string]any); ok && m["schema"] == LegacySchemaID {
		id = LegacySchemaID
	}
	if err := reg.Validate(id, doc); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Change
	if err := dec.Decode(&c); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode change", err)
	}
	c.Schema = SchemaID
	return &c, nil
}

// IsLegacy reports whether data is an older change schema that `athel migrate` would rewrite.
func IsLegacy(data []byte) bool {
	doc, err := schema.DecodeYAML(data)
	m, ok := doc.(map[string]any)
	return err == nil && ok && m["schema"] == LegacySchemaID
}

// Encode renders c as YAML and validates the result, so an invalid change is never written.
func Encode(reg *schema.Registry, c *Change) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "encode change "+c.ID, err)
	}
	if err := enc.Close(); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "encode change "+c.ID, err)
	}
	if err := reg.ValidateYAML(SchemaID, buf.Bytes()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Load reads and validates the change file at path.
func Load(reg *schema.Registry, path string) (*Change, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is built from the workspace root and a validated change ID
	if errors.Is(err, fs.ErrNotExist) {
		return nil, aerr.Wrap(aerr.StateNotFound, "no change file at "+path, err)
	}
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	c, err := Decode(reg, data)
	if err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, path+" is invalid", err)
	}
	return c, nil
}

// Save validates c and atomically replaces the file at path.
func Save(reg *schema.Registry, path string, c *Change) error {
	data, err := Encode(reg, c)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data)
}
