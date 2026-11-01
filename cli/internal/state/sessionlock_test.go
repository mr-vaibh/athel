package state

import (
	"strings"
	"testing"
	"time"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestCheckSession(t *testing.T) {
	tests := []struct {
		in   string
		code aerr.Code
	}{
		{"abc-123_x.y:z", ""},
		{"", aerr.StateNoSession},
		{"has space", aerr.StateInvalid},
		{strings.Repeat("a", 201), aerr.StateInvalid},
	}
	for _, tt := range tests {
		err := CheckSession(tt.in)
		if (tt.code == "" && err != nil) || (tt.code != "" && !aerr.HasCode(err, tt.code)) {
			t.Errorf("CheckSession(%q) = %v, want %s", tt.in, err, tt.code)
		}
	}
}

func TestAcquireFree(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	got, err := s.AcquireLock(c.ID, "A", AcquireOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Lock == nil || got.Lock.Session != "A" || got.Lock.TTLMin != 30 || got.Lock.Since != got.Lock.HeartbeatAt {
		t.Errorf("lock = %+v", got.Lock)
	}
	again, err := s.AcquireLock(c.ID, "A", AcquireOptions{})
	if err != nil || again.Lock.Since != got.Lock.Since {
		t.Errorf("re-acquire by holder: %+v, %v", again.Lock, err)
	}
}

func TestSecondSessionReadOnly(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err := s.AcquireLock(c.ID, "B", AcquireOptions{})
	if !aerr.HasCode(err, aerr.StateLockHeld) || !strings.Contains(err.Error(), "session A") {
		t.Fatalf("err = %v, want E-STATE-LOCK-HELD naming A", err)
	}
	if _, err := s.AcquireLock(c.ID, "B", AcquireOptions{Takeover: true, Reason: "impatient"}); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("take-over of a live lock: err = %v", err)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{To: Investigating, By: ByHuman, Reason: "r", Session: ptr("B")}); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("write from B: err = %v", err)
	}
	if got, err := s.Get(c.ID); err != nil || got.State != Draft {
		t.Errorf("read from B: %v %v", got, err)
	}
	if _, err := s.ReleaseLock(c.ID, "B"); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("release by non-holder: err = %v", err)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{To: Investigating, By: ByHuman, Reason: "r", Session: ptr("A")}); err != nil {
		t.Errorf("write from holder: %v", err)
	}
}

func TestHeartbeatExtendsAndIsThrottled(t *testing.T) {
	s, clk := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Minute)
	if wrote, err := s.RefreshLock(c.ID, "A", false); err != nil || wrote {
		t.Errorf("refresh inside the interval: wrote=%v err=%v", wrote, err)
	}
	if wrote, err := s.RefreshLock(c.ID, "A", true); err != nil || !wrote {
		t.Errorf("forced refresh: wrote=%v err=%v", wrote, err)
	}
	clk.Advance(6 * time.Minute)
	if wrote, err := s.RefreshLock(c.ID, "A", false); err != nil || !wrote {
		t.Errorf("refresh after the interval: wrote=%v err=%v", wrote, err)
	}
	clk.Advance(29 * time.Minute)
	if st := Status(mustGet(t, s, c.ID), clk.Now()); st.Stale {
		t.Errorf("lock went stale despite heartbeats: %+v", st)
	}
}

func TestRefreshErrors(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.RefreshLock(c.ID, "A", false); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("refresh without lock: err = %v", err)
	}
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RefreshLock(c.ID, "B", false); !aerr.HasCode(err, aerr.StateLockHeld) {
		t.Errorf("refresh by other: err = %v", err)
	}
	if _, err := s.RefreshLock(c.ID, "", false); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: err = %v", err)
	}
	if _, err := s.RefreshLock("CHG-404", "A", false); !aerr.HasCode(err, aerr.StateNotFound) {
		t.Errorf("missing change: err = %v", err)
	}
}

func TestStaleAfterTTL(t *testing.T) {
	s, clk := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(30 * time.Minute)
	if st := Status(mustGet(t, s, c.ID), clk.Now()); st.Stale || st.StaleAt != "2026-11-01T10:30:00.000Z" {
		t.Errorf("at exactly TTL: %+v", st)
	}
	clk.Advance(time.Millisecond)
	if st := Status(mustGet(t, s, c.ID), clk.Now()); !st.Stale {
		t.Errorf("after TTL: %+v", st)
	}
	broken := &Change{ID: "CHG-009", Lock: &Lock{Session: "A", HeartbeatAt: "garbage", TTLMin: 30}}
	if !Status(broken, clk.Now()).Stale {
		t.Error("unparseable heartbeat must count as stale")
	}
	if st := Status(&Change{ID: "CHG-010"}, clk.Now()); st.Held || st.Stale {
		t.Errorf("free lock status = %+v", st)
	}
}

func TestTakeoverOnlyWhenStaleAndLogged(t *testing.T) {
	s, clk := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(31 * time.Minute)
	if _, err := s.AcquireLock(c.ID, "B", AcquireOptions{}); !aerr.HasCode(err, aerr.StateLockHeld) || !strings.Contains(err.Error(), "stale") {
		t.Errorf("stale without --takeover: err = %v", err)
	}
	if _, err := s.AcquireLock(c.ID, "B", AcquireOptions{Takeover: true}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("take-over without reason: err = %v", err)
	}
	got, err := s.AcquireLock(c.ID, "B", AcquireOptions{Takeover: true, Reason: "session A crashed"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Lock.Session != "B" || got.Lock.Since != "2026-11-01T10:31:00.000Z" {
		t.Errorf("lock after take-over = %+v", got.Lock)
	}
	events := readStateEvents(t, s)
	last := events[len(events)-1]
	if last.Kind != EventLockTakeover || *last.Session != "B" || !strings.Contains(*last.Reason, "session A crashed") || !strings.Contains(*last.Reason, "session A ") {
		t.Errorf("take-over event = %+v", last)
	}
}

func TestReleaseOnlyByHolder(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	if _, err := s.ReleaseLock(c.ID, "A"); err != nil {
		t.Errorf("release of a free lock: %v", err)
	}
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReleaseLock(c.ID, "A")
	if err != nil || got.Lock != nil {
		t.Errorf("release by holder: %+v %v", got, err)
	}
	if _, err := s.ReleaseLock(c.ID, ""); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: err = %v", err)
	}
}

func TestReleaseAllMine(t *testing.T) {
	s, _ := newTestStore(t)
	a1, a2, b := mustCreate(t, s, "a1"), mustCreate(t, s, "a2"), mustCreate(t, s, "b")
	mustCreate(t, s, "free")
	for _, x := range []struct{ id, sess string }{{a1.ID, "A"}, {a2.ID, "A"}, {b.ID, "B"}} {
		if _, err := s.AcquireLock(x.id, x.sess, AcquireOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	released, err := s.ReleaseAllMine("A")
	if err != nil || strings.Join(released, ",") != "CHG-001,CHG-002" {
		t.Errorf("released %v, %v", released, err)
	}
	if mustGet(t, s, b.ID).Lock == nil {
		t.Error("B's lock was released")
	}
	if _, err := s.ReleaseAllMine(""); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("no session: err = %v", err)
	}
}

func TestLockRejectedOnTerminalAndReleasedOnFinish(t *testing.T) {
	s, _ := newTestStore(t)
	c := mustCreate(t, s, "x")
	walk(t, s, c.ID, Cancelled)
	if got := mustGet(t, s, c.ID); got.Lock != nil {
		t.Errorf("lock kept after cancel: %+v", got.Lock)
	}
	if _, err := s.AcquireLock(c.ID, "A", AcquireOptions{}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("lock on cancelled change: err = %v", err)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{To: Draft, By: ByHuman, Reason: "r"}); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("transition without session: err = %v", err)
	}
	if _, err := s.Transition(c.ID, TransitionRequest{To: Draft, By: ByHuman, Reason: "r", Session: ptr("bad session")}); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("transition with invalid session: err = %v", err)
	}
	if _, err := s.AcquireLock(c.ID, "", AcquireOptions{}); !aerr.HasCode(err, aerr.StateNoSession) {
		t.Errorf("acquire without session: err = %v", err)
	}
}

func TestStoreTunables(t *testing.T) {
	s, clk := newTestStore(t)
	s.TTLMin, s.HeartbeatEvery = 10, time.Minute
	c := mustCreate(t, s, "x")
	got, err := s.AcquireLock(c.ID, "A", AcquireOptions{})
	if err != nil || got.Lock.TTLMin != 10 {
		t.Fatalf("ttl = %+v, %v", got.Lock, err)
	}
	clk.Advance(61 * time.Second)
	if wrote, err := s.RefreshLock(c.ID, "A", false); err != nil || !wrote {
		t.Errorf("custom heartbeat interval: wrote=%v err=%v", wrote, err)
	}
}

func mustGet(t *testing.T, s *Store, id string) *Change {
	t.Helper()
	c, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
