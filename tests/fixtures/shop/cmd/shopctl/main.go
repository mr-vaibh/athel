// Command shopctl is the shop fixture's fake kubectl, gcloud, gh and praxis: link or copy it
// under those names on a scenario-only PATH. SHOP_SOURCES names the fixture's source trees, and
// SHOP_GH_STATE the folder where the fake GitHub keeps pull requests and its call log; when it is
// unset, gh's own GH_CONFIG_DIR, which Athel passes to gh (its other settings are scrubbed).
package main

import (
	"fmt"
	"os"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
	"github.com/mr-vaibh/athel/tests/fixtures/shop/fakes"
)

// exit is a variable so the entry point can be tested in-process.
var exit = os.Exit

func main() { exit(run(os.Args, os.Getenv, fakes.Load)) }

func run(argv []string, getenv func(string) string, load func() (*fakes.Data, error)) int {
	data, err := load()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: "+err.Error())
		return 1
	}
	sources := getenv("SHOP_SOURCES")
	if sources == "" {
		sources = shop.Sources()
	}
	state := getenv("SHOP_GH_STATE")
	if state == "" {
		state = getenv("GH_CONFIG_DIR")
	}
	return fakes.Shim{Data: data, Sources: sources, State: state, Out: os.Stdout, Err: os.Stderr}.Run(argv)
}
