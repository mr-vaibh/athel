// Package api serves /v1/users. The legacy profile endpoint was removed in 2026-09.
package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"shop/users/events"
)

// User is a customer profile. Phone is owned here: users writes it, everyone else reads it.
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// Register adds the users routes.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/users/{id}", getUser)
	mux.HandleFunc("PUT /v1/users/{id}", putUser)
	mux.HandleFunc("PUT /v1/users/{id}/phone", putPhone)
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

// putPhone stores a phone number in E.164 (eval PRD 1).
func putPhone(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !e164.MatchString(body.Phone) {
		http.Error(w, `{"code":"PHONE_INVALID"}`, http.StatusBadRequest)
		return
	}
	events.PublishUserUpdated(r.PathValue("id"), body.Phone)
	_ = json.NewEncoder(w).Encode(User{ID: r.PathValue("id"), Phone: body.Phone})
}
