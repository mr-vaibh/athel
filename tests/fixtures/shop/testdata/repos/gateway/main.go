// Package main is the gateway: it routes by path prefix to the services in routes.yaml.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// routes mirrors routes.yaml; the cluster's gateway config is the source of truth.
var routes = map[string]string{"/payments/": "http://payments:8080", "/users/": "http://user-svc:8080"}

func main() {
	mux := http.NewServeMux()
	for prefix, target := range routes {
		u, err := url.Parse(target)
		if err != nil {
			log.Fatal(err)
		}
		mux.Handle(prefix, http.StripPrefix(prefix[:len(prefix)-1], httputil.NewSingleHostReverseProxy(u)))
	}
	log.Fatal(http.ListenAndServe(":8080", mux))
}
