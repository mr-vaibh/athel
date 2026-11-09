// Package api serves /v1/users. The legacy profile endpoint was removed in 2026-09.
package api

import (
	"encoding/json"
	"net/http"

	"shop/users/events"
)

// User is a customer profile. Phone is owned here: users writes it, everyone else reads it.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Register adds the users routes.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/users/{id}", getUser)
	mux.HandleFunc("PUT /v1/users/{id}", putUser)
}

func getUser(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{ID: r.PathValue("id")})
}

func putUser(w http.ResponseWriter, r *http.Request) {
	var u User
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u.ID = r.PathValue("id")
	events.PublishUserUpdated(u.ID, u.Phone) // writes users.users.phone, then announces it
	_ = json.NewEncoder(w).Encode(u)
}
