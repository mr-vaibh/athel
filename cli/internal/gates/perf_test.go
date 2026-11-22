package gates

import (
	"strings"
	"testing"
	"time"
)

func TestBudget(t *testing.T) {
	for notes, want := range map[string]time.Duration{
		"latency budget 200µs per call": 200 * time.Microsecond,
		"Latency budget: 1.5ms":         1500 * time.Microsecond,
		"latency budget 30us":           30 * time.Microsecond,
		"latency budget of 2s":          2 * time.Second,
		"latency budget 900ns":          900 * time.Nanosecond,
	} {
		if got, ok := Budget(notes); !ok || got != want {
			t.Errorf("%q: %v %v, want %v", notes, got, ok, want)
		}
	}
	for _, notes := range []string{"", "touches the payments path", "budget 5ms", "latency budget soon"} {
		if _, ok := Budget(notes); ok {
			t.Errorf("%q names no latency budget", notes)
		}
	}
}

const benchOut = `goos: darwin
pkg: example.com/users/src
BenchmarkFast-8   	 1000000	      1050 ns/op	       0 B/op	       0 allocs/op
BenchmarkSlow-8   	      10	 101234567 ns/op
PASS
pkg: example.com/users/api
BenchmarkTiny-8   	1000000000	         0.25 ns/op
ok  	example.com/users/src	2.1s
`

func TestParseBench(t *testing.T) {
	got := ParseBench(benchOut)
	if len(got) != 3 || got[0].Name != "example.com/users/src BenchmarkFast" || got[0].NsPerOp != 1050 || got[2].NsPerOp != 0.25 {
		t.Errorf("%+v", got)
	}
	over := OverBudget(got, time.Millisecond)
	if len(over) != 1 || !strings.Contains(over[0], "BenchmarkSlow") || !strings.Contains(over[0], "101.2ms") {
		t.Errorf("%q", over)
	}
}
