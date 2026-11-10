package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestRun(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"shopmcp"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Errorf("usage: %d", code)
	}
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	if code := run([]string{"shopmcp", "grafana"}, strings.NewReader(in), &out, &errOut); code != 0 || !strings.Contains(out.String(), "query_prometheus") {
		t.Errorf("serve: %d %s", code, out.String())
	}
	if code := run([]string{"shopmcp", "grafana"}, strings.NewReader(in), failWriter{}, &errOut); code != 1 {
		t.Errorf("write failure: %d", code)
	}
	origArgs, origExit := os.Args, exit
	t.Cleanup(func() { os.Args, exit = origArgs, origExit })
	got := -1
	exit = func(code int) { got = code }
	os.Args = []string{"shopmcp"}
	main()
	if got != 2 {
		t.Errorf("main exit = %d", got)
	}
}
