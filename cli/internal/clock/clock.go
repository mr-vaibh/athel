// Package clock injects time so every timestamp comes from the CLI and tests are reproducible.
package clock

import (
	"sync"
	"time"
)

// Layout is the single timestamp format in every Athel file: RFC 3339, UTC, milliseconds.
const Layout = "2006-01-02T15:04:05.000Z07:00"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the real clock, always in UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a test clock that only moves when told to. It is safe for concurrent use.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed returns a Fixed clock set to t (converted to UTC).
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t.UTC()} }

// Now returns the clock's current time.
func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance moves the clock forward by d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// Format renders t in Layout, in UTC.
func Format(t time.Time) string { return t.UTC().Format(Layout) }

// Parse reads a timestamp written by Format (any RFC 3339 value is accepted).
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
