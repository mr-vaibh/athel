package decisions

import (
	"encoding/json"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const at = "2026-11-10T10:00:00.000Z"

func card(q string) Draft {
	return Draft{Rule: "A1", Step: "spec", Question: q, Context: "PRD-2.1 says valid phone number.",
		Options:        []Option{{"a", "E.164 only", "Clean data."}, {"b", "E.164 plus local formats", "Friendlier."}},
		Recommendation: Recommendation{"b", "the app sends local formats"}, Default: "b", DefaultSafe: true, Blocks: []string{"T-03"}}
}

func TestAddAssignsIDsAndRoundTrips(t *testing.T) {
	reg := schema.NewRegistry()
	f := New("CHG-001")
	c1, err := f.Add(card("Which phone formats?"))
	if err != nil || c1.ID != "D-1" || c1.Kind != "card" {
		t.Fatalf("add: %+v %v", c1, err)
	}
	d := f.Record(DecidedDraft{Step: "plan", Question: "Where does the validator live?", Choice: "src/validation", Reason: "conventions"}, at)
	if d.ID != "D-2" || d.DecidedBy != "athel" {
		t.Errorf("record: %+v", d)
	}
	c3, _ := f.Add(card("Keep the old field?"))
	if c3.ID != "D-3" {
		t.Errorf("third: %s", c3.ID)
	}
	checkRoundTrip(t, reg, f)
}

func checkRoundTrip(t *testing.T, reg *schema.Registry, f *File) {
	t.Helper()
	b, err := f.Encode(reg)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Decode(reg, "CHG-001", b)
	if err != nil || len(back.Decisions) != 3 || back.Decisions[1].Decided == nil || back.Decisions[2].ID() != "D-3" {
		t.Fatalf("decode: %+v %v", back, err)
	}
	if got, ok := back.Card("D-1"); !ok || got.Recommendation.Option != "b" {
		t.Errorf("card: %+v", got)
	}
	if _, ok := back.Card("D-2"); ok {
		t.Error("D-2 is a decision, not a card")
	}
	j, _ := json.Marshal(back.Decisions)
	if !strings.Contains(string(j), `"decided_by":"athel"`) || !strings.Contains(string(j), `"rule":"A1"`) {
		t.Errorf("json: %s", j)
	}
}

func TestDecodeEmptyAndInvalid(t *testing.T) {
	reg := schema.NewRegistry()
	if f, err := Decode(reg, "CHG-001", nil); err != nil || f.Change != "CHG-001" || len(f.Decisions) != 0 {
		t.Errorf("empty: %+v %v", f, err)
	}
	if _, err := Decode(reg, "CHG-001", []byte("schema: athel/decision@1\nchange: CHG-001\ndecisions: [{kind: card}]\n")); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("invalid: %v", err)
	}
	bad := New("CHG-001")
	bad.Decisions = append(bad.Decisions, Entry{Card: &Card{ID: "D-1", Kind: "card"}})
	if _, err := bad.Encode(reg); !aerr.HasCode(err, aerr.StateSchema) {
		t.Errorf("an invalid file is never stored: %v", err)
	}
}

func TestCardLint(t *testing.T) {
	f := New("CHG-001")
	if _, err := f.Add(card("Which phone formats?")); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		edit func(*Draft)
		want string
	}{
		"keys out of order":       {func(c *Draft) { c.Options[1].Key = "c" }, `option 2 has key "c"`},
		"unknown recommendation":  {func(c *Draft) { c.Recommendation.Option = "d" }, "the recommendation names option d"},
		"unknown default":         {func(c *Draft) { c.Default = "c" }, "default_if_unanswered names option c"},
		"duplicate open question": {func(c *Draft) { c.Question = "which phone formats?" }, "open card D-1 already asks"},
	}
	for name, tt := range tests {
		c := card("A new question?")
		tt.edit(&c)
		if _, err := f.Add(c); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestPresentOneInterruptionPerStep(t *testing.T) {
	f := New("CHG-001")
	long, _ := f.Add(card("Which phone number formats should the API accept?"))
	short, _ := f.Add(card("Keep the old field?"))
	other := card("Which team owns it?")
	other.Step = "plan"
	if _, err := f.Add(other); err != nil {
		t.Fatal(err)
	}
	batch, err := f.Present("spec", at)
	if err != nil || len(batch) != 2 || batch[0].ID != short.ID || batch[1].ID != long.ID || *long.PresentedAt != at {
		t.Fatalf("batch: %+v %v", batch, err)
	}
	if again, err := f.Present("spec", "2026-11-10T11:00:00.000Z"); err != nil || len(again) != 2 || *again[0].PresentedAt != at {
		t.Errorf("showing the same batch again is fine: %v", err)
	}
	if _, err := f.Add(card("A late question?")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Present("spec", at); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), "D-4 may join only as a new blocker") {
		t.Errorf("a second interruption: %v", err)
	}
	checkNewBlocker(t, f)
}

// checkNewBlocker: after a step's batch, a card flagged as a new blocker may interrupt again.
func checkNewBlocker(t *testing.T, f *File) {
	t.Helper()
	blocker := card("A new blocker?")
	blocker.NewBlocker = true
	f.Decisions = f.Decisions[:3]
	if _, err := f.Add(blocker); err != nil {
		t.Fatal(err)
	}
	if batch, err := f.Present("spec", at); err != nil || len(batch) != 3 {
		t.Errorf("a new blocker may interrupt again: %v %v", batch, err)
	}
	if open := f.Open(""); len(open) != 4 {
		t.Errorf("open across steps: %d", len(open))
	}
}
