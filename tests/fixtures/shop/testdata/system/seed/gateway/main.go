// Package main is the gateway: it routes by path prefix to the services in routes.yaml.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// routes mirrors routes.yaml; the cluster's gateway config is the source of truth.
var routes = map[string]string{"/payments/": "http://payments:8080", "/users/": "http://user-svc:8080"}

func main() {
	mux := http.NewServeMux()
	for prefix, target := range routes {
		u, err := url.Parse(routeTo(os.Getenv, prefix, target))
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle(prefix, http.StripPrefix(prefix[:len(prefix)-1], httputil.NewSingleHostReverseProxy(u)))
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(http.ListenAndServe(addr, mux)) //nolint:gosec // fixture server, run only by local checks
}

// routeTo lets a local run point a route elsewhere: ROUTE_USERS overrides /users/.
func routeTo(getenv func(string) string, prefix, fallback string) string {
	if v := getenv("ROUTE_" + strings.ToUpper(strings.Trim(prefix, "/"))); v != "" {
		return v
	}
	return fallback
}
