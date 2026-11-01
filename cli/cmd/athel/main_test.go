package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestMainExitsWithRunCode(t *testing.T) {
	origArgs, origExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = origArgs, origExit })
	os.Args = []string{"athel", "unknown-command"}
	got := -1
	exit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main exit code = %d, want 2", got)
	}
}

func TestRunWriteFailure(t *testing.T) {
	if got := run([]string{"version"}, failingWriter{}, failingWriter{}); got != 1 {
		t.Errorf("version with failing stdout: exit code = %d, want 1", got)
	}
	if got := run(nil, failingWriter{}, failingWriter{}); got != 1 {
		t.Errorf("usage with failing stderr: exit code = %d, want 1", got)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "version", args: []string{"version"}, wantCode: 0, wantStdout: "athel 0.0.0-dev"},
		{name: "no args", args: nil, wantCode: 2, wantStderr: "usage:"},
		{name: "unknown command", args: []string{"nope"}, wantCode: 2, wantStderr: "usage:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tt.args, &stdout, &stderr)
			if got != tt.wantCode {
				t.Fatalf("exit code = %d, want %d", got, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
