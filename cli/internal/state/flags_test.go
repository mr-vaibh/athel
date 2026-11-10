package state

import (
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestFlagFact(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	f := Flag{Kind: FlagFactDisputed, Fact: "F-0001", At: "2026-11-09T10:00:00.000Z", Reason: "consumer group gone"}
	if added, err := s.FlagFact(c.ID, f); err != nil || !added {
		t.Fatalf("flag = %v %v", added, err)
	}
	if added, err := s.FlagFact(c.ID, f); err != nil || added {
		t.Errorf("repeat flag = %v %v", added, err)
	}
	f.Kind = FlagFactSuperseded
	if added, _ := s.FlagFact(c.ID, f); !added {
		t.Error("a different kind for the same fact is a new flag")
	}
	read, _ := s.Get(c.ID)
	if len(read.Flags) != 2 {
		t.Errorf("flags = %+v", read.Flags)
	}
	done := mustCreate(t, s, "y")
	if _, err := s.Update(done.ID, func(c *Change) error { c.State = Cancelled; return nil }); err != nil {
		t.Fatal(err)
	}
	if added, err := s.FlagFact(done.ID, f); err != nil || added {
		t.Errorf("flag on a cancelled change = %v %v", added, err)
	}
	if _, err := s.FlagFact("CHG-099", f); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("unknown change: %v", err)
	}
}
