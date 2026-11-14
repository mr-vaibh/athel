package gates

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Doc 06's performance gate: when a task's risk_notes names a latency budget, its packages'
// benchmarks must stay within it ("a benchmark or load test gate is added for that task").

var budget = regexp.MustCompile(`(?i)latency budget\D*?(\d+(?:\.\d+)?)\s*(ns|µs|us|ms|s)\b`)

var units = map[string]time.Duration{"ns": time.Nanosecond, "µs": time.Microsecond, "us": time.Microsecond, "ms": time.Millisecond, "s": time.Second}

// Budget reads a latency budget from risk_notes, such as "latency budget 200µs".
func Budget(notes string) (time.Duration, bool) {
	m := budget.FindStringSubmatch(notes)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.ParseFloat(m[1], 64) // the pattern has a number
	return time.Duration(n * float64(units[m[2]])), true
}

// Bench is one benchmark result.
type Bench struct {
	Name    string // "package BenchmarkName"
	NsPerOp float64
}

var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+(\d+(?:\.\d+)?) ns/op`)

// ParseBench reads `go test -bench` output.
func ParseBench(out string) []Bench {
	var got []Bench
	pkg := ""
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(line, "pkg: "); ok {
			pkg = p
			continue
		}
		if m := benchLine.FindStringSubmatch(line); m != nil {
			ns, _ := strconv.ParseFloat(m[2], 64) // the pattern has a number
			got = append(got, Bench{Name: pkg + " " + m[1], NsPerOp: ns})
		}
	}
	return got
}

// OverBudget lists the benchmarks slower than the budget per operation.
func OverBudget(benches []Bench, limit time.Duration) []string {
	var out []string
	for _, b := range benches {
		if took := time.Duration(b.NsPerOp); took > limit {
			out = append(out, fmt.Sprintf("%s takes %v per operation; the budget is %v", b.Name, took.Round(100*time.Microsecond), limit))
		}
	}
	return out
}
