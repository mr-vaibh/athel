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

// Decode validates data against change@1 and decodes it. Validation runs on the generic
// document first, so a file that does not match the schema never reaches the typed model.
func Decode(reg *schema.Registry, data []byte) (*Change, error) {
	if err := reg.ValidateYAML(SchemaID, data); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Change
	if err := dec.Decode(&c); err != nil {
		return nil, aerr.Wrap(aerr.StateSchema, "decode change", err)
	}
	return &c, nil
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
