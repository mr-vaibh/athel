package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestAtomicWriteReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "change.yaml")
	for _, content := range []string{"first\n", "second, longer content\n", ""} {
		if err := WriteFileAtomic(path, []byte(content)); err != nil {
			t.Fatalf("WriteFileAtomic(%q): %v", content, err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != PrivateFile {
		t.Errorf("mode = %v, want %v", info.Mode().Perm(), PrivateFile)
	}
	assertNoTempFiles(t, dir)
}

func TestAtomicWriteNoPartialOnFailure(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{
			name: "directory missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "change.yaml")
			},
		},
		{
			name: "target is a non-empty directory",
			setup: func(t *testing.T) string {
				target := filepath.Join(t.TempDir(), "change.yaml")
				if err := os.MkdirAll(filepath.Join(target, "child"), 0o700); err != nil {
					t.Fatal(err)
				}
				return target
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			err := WriteFileAtomic(path, []byte("data"))
			if !aerr.HasCode(err, aerr.StateIO) {
				t.Fatalf("err = %v, want E-STATE-IO", err)
			}
			if _, statErr := os.Stat(filepath.Dir(path)); statErr == nil {
				assertNoTempFiles(t, filepath.Dir(path))
			}
		})
	}
}

func TestAppendLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	for _, line := range []string{`{"seq":1}`, `{"seq":2}`} {
		if err := AppendLine(path, []byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\"seq\":1}\n{\"seq\":2}\n"; string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestAppendLineErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		path string
		line string
		code aerr.Code
	}{
		{"newline in record", filepath.Join(dir, "a.jsonl"), "a\nb", aerr.StateInvalid},
		{"directory missing", filepath.Join(dir, "missing", "a.jsonl"), "a", aerr.StateIO},
		{"path is a directory", dir, "a", aerr.StateIO},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := AppendLine(tt.path, []byte(tt.line)); !aerr.HasCode(err, tt.code) {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
		})
	}
}

func TestSyncDirMissing(t *testing.T) {
	if err := SyncDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("SyncDir on a missing directory succeeded")
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("second EnsureDir: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(file, "sub")); !aerr.HasCode(err, aerr.StateIO) {
		t.Errorf("EnsureDir under a file: err = %v, want E-STATE-IO", err)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}
