// Package main is the users service: customer profiles, including the phone number.
package main

import (
	"log"
	"net/http"
	"os"

	"shop/users/api"
)

func main() {
	mux := http.NewServeMux()
	api.Register(mux)
	log.Fatal(http.ListenAndServe(addr(os.Getenv), mux)) //nolint:gosec // fixture server, run only by local checks
}

// addr is where the service listens: ADDR when set (local runs), otherwise the cluster's :8080.
func addr(getenv func(string) string) string {
	if a := getenv("ADDR"); a != "" {
		return a
	}
	return ":8080"
}
