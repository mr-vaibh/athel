// Command shopctl is the shop fixture's fake kubectl, gcloud, gh and praxis: link or copy it
// under those names on a scenario-only PATH. SHOP_SOURCES names the fixture's source trees.
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
	return fakes.Shim{Data: data, Sources: sources, Out: os.Stdout, Err: os.Stderr}.Run(argv)
}
