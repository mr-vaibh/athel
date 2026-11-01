package state

import (
	"testing"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func newTestStore(t testing.TB) (*Store, *clock.Fixed) {
	t.Helper()
	ws, _, err := Init(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewFixed(t0)
	return NewStore(ws, schema.NewRegistry(), clk), clk
}

func mustCreate(t testing.TB, s *Store, title string) *Change {
	t.Helper()
	c, err := s.Create(NewChange{
		Title: title, CreatedBy: "dev@example.com", PRD: []byte("# PRD\n"), PRDSource: "text",
		Budget: Budget{MaxCostUSD: 40, MaxHours: 8, MaxToolCalls: 2000},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
