package facts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/fsutil"
)

func encodeLine(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// appendRaw appends a record as a later CLI write would, bypassing Add's checks.
func appendRaw(t testing.TB, s *Store, f Fact) {
	t.Helper()
	appendLine(t, s, string(encodeLine(t, f)))
}

func appendLine(t testing.TB, s *Store, line string) {
	t.Helper()
	if err := fsutil.AppendLine(filepath.Join(s.Dir, "facts.jsonl"), []byte(strings.TrimSuffix(line, "\n"))); err != nil {
		t.Fatal(err)
	}
}

func readAccess(t testing.TB, s *Store) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.Dir, "access.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}
