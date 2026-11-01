package state

import (
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestDimensionStatus(t *testing.T) {
	tests := []struct {
		used, limit float64
		want        string
	}{
		{0, 40, BudgetOK},
		{31.6, 40, BudgetOK},      // 79%
		{32, 40, BudgetWarning},   // 80%
		{39.6, 40, BudgetWarning}, // 99%
		{40, 40, BudgetExceeded},  // 100%
		{48, 40, BudgetExceeded},  // 120%
		{1000, 0, BudgetOK},       // no limit
	}
	for _, tt := range tests {
		if got := dimensionStatus(tt.used, tt.limit, DefaultWarnRatio); got != tt.want {
			t.Errorf("dimensionStatus(%v/%v) = %s, want %s", tt.used, tt.limit, got, tt.want)
		}
	}
}

func TestBudgetThresholds(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x") // limits: $40, 8 h, 2000 tool calls
	steps := []struct {
		usage      Usage
		wantStatus string
		wantEvents int
	}{
		{Usage{CostUSD: 10}, BudgetOK, 0},
		{Usage{CostUSD: 22}, BudgetWarning, 1},                 // $32 = 80%
		{Usage{Minutes: 100, ToolCalls: 10}, BudgetWarning, 1}, // no new crossing
		{Usage{ToolCalls: 1600}, BudgetWarning, 2},             // tool calls 1610 = 80.5%
		{Usage{CostUSD: 8}, BudgetExceeded, 3},                 // $40 = 100%
		{Usage{Minutes: 400}, BudgetExceeded, 4},               // 500 of 480 min: ok jumps straight to exceeded
	}
	for i, st := range steps {
		got, err := s.RecordUsage(c.ID, testSession, st.usage)
		if err != nil {
			t.Fatal(err)
		}
		if got.Budget.Status != st.wantStatus {
			t.Errorf("step %d: status %s, want %s", i, got.Budget.Status, st.wantStatus)
		}
		if len(got.Budget.Events) != st.wantEvents {
			t.Errorf("step %d: %d events, want %d: %+v", i, len(got.Budget.Events), st.wantEvents, got.Budget.Events)
		}
	}
	final := mustGet(t, s, c.ID)
	last := final.Budget.Events[len(final.Budget.Events)-1]
	if last.Dimension != "time" || last.Status != BudgetExceeded || len(final.Budget.Events) != 4 {
		t.Errorf("events = %+v", final.Budget.Events)
	}
}

func TestRecordUsageErrors(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.RecordUsage(c.ID, testSession, Usage{CostUSD: -1}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("negative: err = %v", err)
	}
	if _, err := s.RecordUsage(c.ID, "", Usage{}); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: err = %v", err)
	}
	if _, err := s.RecordUsage(c.ID, testSession, Usage{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordUsage(c.ID, "other", Usage{}); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("other session: err = %v", err)
	}
}

func TestCustomWarnRatio(t *testing.T) {
	s, _ := newTestStore(t)
	s.WarnRatio = 0.5
	c := mustCreate(t, s, "x")
	got, err := s.RecordUsage(c.ID, testSession, Usage{CostUSD: 20})
	if err != nil || got.Budget.Status != BudgetWarning {
		t.Errorf("50%% with ratio 0.5: %s %v", got.Budget.Status, err)
	}
}
