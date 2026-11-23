// Command shopsys runs the shop's system check (package systest) on service folders:
//
//	shopsys --flows flows.yaml [--users DIR] [--orders DIR] [--gateway DIR]
//
// It prints one JSON object with every flow's result. Exit 0 when every flow passes, 1 when one
// fails, 2 when the check could not run. A workspace names it as its system check (M5-plan A1).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mr-vaibh/athel/tests/fixtures/shop/systest"
)

// exit is a variable so the entry point can be tested in-process.
var exit = os.Exit

func main() { exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(argv []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("shopsys", flag.ContinueOnError)
	fs.SetOutput(errOut)
	flows := fs.String("flows", "", "the flows file")
	dirs := map[string]*string{}
	for _, name := range systest.Services {
		dirs[name] = fs.String(name, "", "the "+name+" service folder")
	}
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if *flows == "" {
		_, _ = fmt.Fprintln(errOut, "error: --flows is required")
		return 2
	}
	f, err := systest.LoadFlows(*flows)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error: "+err.Error())
		return 2
	}
	services := map[string]string{}
	for name, dir := range dirs {
		if *dir != "" {
			services[name] = *dir
		}
	}
	results, err := systest.Run(context.Background(), services, f)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "error: "+err.Error())
		return 2
	}
	failed := systest.Failed(results)
	_ = json.NewEncoder(out).Encode(map[string]any{"ok": len(failed) == 0, "results": results}) // plain values always encode
	if len(failed) > 0 {
		return 1
	}
	return 0
}
