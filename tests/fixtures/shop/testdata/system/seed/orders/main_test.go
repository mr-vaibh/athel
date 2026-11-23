package main

import "testing"

func TestEnv(t *testing.T) {
	tests := []struct {
		name, set, want string
	}{
		{"cluster default", "", "http://user-svc:8080"},
		{"local run", "http://127.0.0.1:9001", "http://127.0.0.1:9001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := env(func(string) string { return tt.set }, "USERS_URL", "http://user-svc:8080"); got != tt.want {
				t.Errorf("env = %q, want %q", got, tt.want)
			}
		})
	}
}
