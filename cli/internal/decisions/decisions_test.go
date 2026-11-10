package decisions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestApplyAnswers(t *testing.T) {
	f := New("CHG-001")
	for _, q := range []string{"Q one?", "Q two?", "Q three?"} {
		if _, err := f.Add(card(q)); err != nil {
			t.Fatal(err)
		}
	}
	err := f.Apply([]Reply{{Card: "D-1", Option: "a"}, {Card: "D-2", Text: "only E.164"}, {Card: "D-2", Skip: true}}, "GR-1", at)
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := f.Card("D-1")
	c2, _ := f.Card("D-2")
	if c1.Answer.Option != "a" || c1.Answer.By != "human" || c1.Answer.Grant != "GR-1" || c1.Answer.Checked != nil {
		t.Errorf("D-1: %+v", c1.Answer)
	}
	if c2.Answer.Text != "only E.164" || c2.Answer.Checked == nil || *c2.Answer.Checked || c2.ShouldNotHaveAsked == nil || !*c2.ShouldNotHaveAsked {
		t.Errorf("D-2: %+v %v", c2.Answer, c2.ShouldNotHaveAsked)
	}
	checkApplyRejects(t, f)
}

// checkApplyRejects: one bad answer rejects the whole batch and changes nothing.
func checkApplyRejects(t *testing.T, f *File) {
	t.Helper()
	for name, tt := range map[string]struct {
		r    Reply
		want string
	}{
		"unknown card":     {Reply{Card: "D-9", Option: "a"}, "D-9 is not a card"},
		"already answered": {Reply{Card: "D-1", Option: "b"}, "D-1 is already answered"},
		"no such option":   {Reply{Card: "D-3", Option: "d"}, "D-3 offers no option d"},
	} {
		if err := f.Apply([]Reply{{Card: "D-3", Option: "a"}, tt.r}, "GR-2", at); !aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: %v", name, err)
		}
		if c3, _ := f.Card("D-3"); c3.Answer != nil {
			t.Fatalf("%s: a rejected batch changes nothing", name)
		}
	}
	if err := f.Apply([]Reply{{Card: "D-1", Skip: true}}, "GR-3", at); err != nil {
		t.Errorf("feedback on an answered card: %v", err)
	}
}

func TestTick(t *testing.T) {
	f := New("CHG-001")
	safe, _ := f.Add(card("Safe?"))
	unsafe := card("Unsafe?")
	unsafe.DefaultSafe = false
	u, _ := f.Add(unsafe)
	if _, err := f.Add(card("Never shown?")); err != nil {
		t.Fatal(err)
	}
	presented := "2026-11-10T08:00:00.000Z"
	safe.PresentedAt, u.PresentedAt = &presented, &presented
	at := func(h, m int) time.Time { return time.Date(2026, 11, 10, h, m, 0, 0, time.UTC) }
	if applied, remind := f.Tick(at(9, 59)); len(applied)+len(remind) != 0 {
		t.Errorf("under 2 h: %v %v", applied, remind)
	}
	if applied, remind := f.Tick(at(10, 0)); len(applied) != 0 || strings.Join(remind, ",") != "D-1,D-2" {
		t.Errorf("at 2 h: %v %v", applied, remind)
	}
	applied, remind := f.Tick(at(12, 0))
	if strings.Join(applied, ",") != "D-1" || strings.Join(remind, ",") != "D-2" {
		t.Errorf("at 4 h: %v %v", applied, remind)
	}
	if safe.Answer == nil || safe.Answer.By != "default" || safe.Answer.Option != "b" || u.Answer != nil {
		t.Errorf("safe default applied, unsafe still blocked: %+v %+v", safe.Answer, u.Answer)
	}
}
