// Command shopmcp is the shop fixture's fake MCP server over stdio. Its only argument names the
// server it plays: grafana, atlassian or shopmetrics.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/mr-vaibh/athel/tests/fixtures/shop/fakes"
)

// exit is a variable so the entry point can be tested in-process.
var exit = os.Exit

func main() { exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr)) }

func run(argv []string, in io.Reader, out, errOut io.Writer) int {
	if len(argv) != 2 {
		_, _ = fmt.Fprintln(errOut, "usage: shopmcp grafana|atlassian|shopmetrics")
		return 2
	}
	data, err := fakes.Load()
	if err == nil {
		err = fakes.Server{Name: argv[1], Data: data}.Serve(in, out)
	}
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error: "+err.Error())
		return 1
	}
	return 0
}
