package main

import "testing"

func TestRouteTo(t *testing.T) {
	tests := []struct {
		name, prefix, set, want string
	}{
		{"cluster default", "/users/", "", "http://user-svc:8080"},
		{"local override", "/users/", "http://127.0.0.1:9001", "http://127.0.0.1:9001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string {
				if k == "ROUTE_USERS" {
					return tt.set
				}
				return ""
			}
			if got := routeTo(getenv, tt.prefix, routes[tt.prefix]); got != tt.want {
				t.Errorf("routeTo = %q, want %q", got, tt.want)
			}
		})
	}
}
