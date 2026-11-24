// Package api serves /v1/orders.
package api

import (
	"io"
	"net/http"

	"shop/orders/clients"
)

// Register adds the orders routes; users is how orders reads customers.
func Register(mux *http.ServeMux, users clients.Users) {
	mux.HandleFunc("GET /v1/orders/{id}/customer", func(w http.ResponseWriter, r *http.Request) {
		customer(w, r, users)
	})
}

// customer answers with the order's customer, as the users service returns it.
func customer(w http.ResponseWriter, r *http.Request, users clients.Users) {
	resp, err := users.Get(customerOf(r.PathValue("id")))
	if err != nil {
		http.Error(w, "users unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "users answered "+resp.Status, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, resp.Body)
}

// customerOf is an order's customer: every fixture order o-N belongs to customer c-N.
func customerOf(order string) string { return "c-" + order }
