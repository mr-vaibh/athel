package clients

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// recorder serves every request with 200 and remembers what it was asked.
func recorder(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+string(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestClientsCallTheDocumentedPaths(t *testing.T) {
	srv, seen := recorder(t)
	calls := []func() (*http.Response, error){
		func() (*http.Response, error) { return Users{Base: srv.URL}.Get("u-1") },
		func() (*http.Response, error) { return Users{Base: srv.URL}.LegacyProfile("u-1") },
		func() (*http.Response, error) { return Payments{Base: srv.URL}.Charge("o-9") },
	}
	for _, call := range calls {
		resp, err := call()
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	want := []string{"GET /v1/users/u-1 ", "GET /v1/users/u-1/legacy-profile ", `POST /payments/charge {"order":"o-9"}`}
	for i, w := range want {
		if (*seen)[i] != w {
			t.Errorf("call %d = %q, want %q", i, (*seen)[i], w)
		}
	}
}
