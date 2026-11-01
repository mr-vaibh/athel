// Package digest computes the hashes Athel records (evidence files, effective config, hook
// inputs). All hashes are SHA-256, written as "sha256:<lowercase hex>".
package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

const prefix = "sha256:"

// Bytes hashes b.
func Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return prefix + hex.EncodeToString(sum[:])
}

// File hashes the file at path, streaming so large evidence files are not loaded in memory.
func File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // evidence paths are resolved and checked by the caller
	if err != nil {
		return "", aerr.Wrap(aerr.StateIO, "open "+path+" for hashing", err)
	}
	defer func() { _ = f.Close() }() // read-only: a close error cannot lose data
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", aerr.Wrap(aerr.StateIO, "read "+path+" for hashing", err)
	}
	return prefix + hex.EncodeToString(h.Sum(nil)), nil
}

// CanonicalJSON hashes the JSON encoding of v. encoding/json sorts map keys, so values built
// from maps and slices hash identically regardless of how they were assembled.
func CanonicalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", aerr.Wrap(aerr.StateInvalid, "encode value for hashing", err)
	}
	return Bytes(b), nil
}
