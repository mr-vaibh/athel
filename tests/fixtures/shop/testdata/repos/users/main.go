// Package main is the users service: customer profiles, including the phone number.
package main

import (
	"log"
	"net/http"

	"shop/users/api"
)

func main() {
	mux := http.NewServeMux()
	api.Register(mux)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
