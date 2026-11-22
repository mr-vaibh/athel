package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestGetUserReturnsTheRequestedID(t *testing.T) {
	rec := serve(t, http.MethodGet, "/v1/users/u-1", "")
	var u User
	if err := json.NewDecoder(rec.Body).Decode(&u); err != nil || u.ID != "u-1" {
		t.Fatalf("got %+v, %v", u, err)
	}
}

func TestPutUser(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantName string
	}{
		{"valid profile", `{"name":"Ada","phone":"+44 20 7946 0000"}`, http.StatusOK, "Ada"},
		{"malformed JSON", `{"name":`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, http.MethodPut, "/v1/users/u-2", tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var u User
			if err := json.NewDecoder(rec.Body).Decode(&u); err != nil || u.ID != "u-2" || u.Name != tt.wantName {
				t.Fatalf("got %+v, %v", u, err)
			}
		})
	}
}
