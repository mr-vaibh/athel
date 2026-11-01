package clock

import (
	"testing"
	"time"
)

func TestSystemIsUTC(t *testing.T) {
	if loc := (System{}).Now().Location(); loc != time.UTC {
		t.Errorf("location = %v, want UTC", loc)
	}
}

func TestFixedAdvance(t *testing.T) {
	start := time.Date(2026, 11, 1, 10, 0, 0, 0, time.FixedZone("IST", 19800))
	c := NewFixed(start)
	if !c.Now().Equal(start) || c.Now().Location() != time.UTC {
		t.Fatalf("Now = %v", c.Now())
	}
	c.Advance(31 * time.Minute)
	if got := c.Now().Sub(start); got != 31*time.Minute {
		t.Errorf("advanced by %v", got)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	tests := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 11, 1, 10, 14, 3, 221_000_000, time.UTC), "2026-11-01T10:14:03.221Z"},
		{time.Date(2026, 11, 1, 10, 14, 3, 0, time.UTC), "2026-11-01T10:14:03.000Z"},
		{time.Date(2026, 11, 1, 15, 44, 3, 0, time.FixedZone("IST", 19800)), "2026-11-01T10:14:03.000Z"},
	}
	for _, tt := range tests {
		got := Format(tt.in)
		if got != tt.want {
			t.Errorf("Format = %q, want %q", got, tt.want)
		}
		back, err := Parse(got)
		if err != nil || !back.Equal(tt.in.Truncate(time.Millisecond)) {
			t.Errorf("Parse(%q) = %v, %v", got, back, err)
		}
	}
	if _, err := Parse("yesterday"); err == nil {
		t.Error("Parse accepted garbage")
	}
}
