// Package main is the orders service.
package main

import (
	"log"
	"net/http"
	"os"

	"shop/orders/api"
	"shop/orders/clients"
)

func main() {
	mux := http.NewServeMux()
	api.Register(mux, clients.Users{Base: env(os.Getenv, "USERS_URL", "http://user-svc:8080")})
	log.Fatal(http.ListenAndServe(env(os.Getenv, "ADDR", ":8080"), mux)) //nolint:gosec // fixture server, run only by local checks
}

// env reads a setting for local runs; the defaults are the cluster's.
func env(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}
