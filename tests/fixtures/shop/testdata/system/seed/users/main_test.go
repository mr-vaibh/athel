package main

import "testing"

func TestAddr(t *testing.T) {
	tests := []struct {
		name, env, want string
	}{
		{"cluster default", "", ":8080"},
		{"local run", "127.0.0.1:9001", "127.0.0.1:9001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addr(func(string) string { return tt.env }); got != tt.want {
				t.Errorf("addr = %q, want %q", got, tt.want)
			}
		})
	}
}
