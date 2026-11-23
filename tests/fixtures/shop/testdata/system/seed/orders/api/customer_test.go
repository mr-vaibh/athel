package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shop/orders/clients"
)

func TestCustomer(t *testing.T) {
	tests := []struct {
		name     string
		users    http.HandlerFunc
		wantCode int
		wantBody string
	}{
		{"asks for the order's customer and passes the user through", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/users/c-7" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`{"id":"c-7"}`))
		}, http.StatusOK, `{"id":"c-7"}`},
		{"users fails", func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}, http.StatusBadGateway, "users answered 404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.users)
			t.Cleanup(srv.Close)
			mux := http.NewServeMux()
			Register(mux, clients.Users{Base: srv.URL})
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders/7/customer", nil))
			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("got %d %q, want %d %q", rec.Code, rec.Body.String(), tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestUsersUnreachable(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, clients.Users{Base: "http://127.0.0.1:1"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/orders/7/customer", nil))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "users unreachable") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}
