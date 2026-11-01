package state

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

// Session lock defaults (doc 02 §Locking; heartbeat throttling is decision Q3).
const (
	DefaultTTLMin         = 30
	DefaultHeartbeatEvery = 5 * time.Minute
)

var sessionRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,200}$`)

// CheckSession validates a Claude Code session ID. Mutating commands need one: the lock must
// name its holder, and the CLI never guesses which session is calling.
func CheckSession(session string) error {
	if session == "" {
		return aerr.New(aerr.StateNoSession, "no session ID: pass --session or set ATHEL_SESSION_ID").
			WithNextAction("skills pass --session ${CLAUDE_SESSION_ID}; hooks receive it from Claude Code")
	}
	if !sessionRe.MatchString(session) {
		return aerr.New(aerr.StateInvalid, fmt.Sprintf("%q is not a valid session ID", session))
	}
	return nil
}

// LockStatus describes a change's session lock as seen at a moment in time.
type LockStatus struct {
	Change      string `json:"change"`
	Held        bool   `json:"held"`
	Session     string `json:"session,omitempty"`
	Since       string `json:"since,omitempty"`
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
	TTLMin      int    `json:"ttl_min,omitempty"`
	StaleAt     string `json:"stale_at,omitempty"`
	Stale       bool   `json:"stale"`
}

// Status reports c's lock at now. A lock whose heartbeat cannot be parsed counts as stale so
// a corrupted lock can always be taken over (with a logged reason).
func Status(c *Change, now time.Time) LockStatus {
	st := LockStatus{Change: c.ID}
	if c.Lock == nil {
		return st
	}
	st.Held, st.Session, st.Since, st.HeartbeatAt, st.TTLMin = true, c.Lock.Session, c.Lock.Since, c.Lock.HeartbeatAt, c.Lock.TTLMin
	hb, err := clock.Parse(c.Lock.HeartbeatAt)
	if err != nil {
		st.Stale = true
		return st
	}
	staleAt := hb.Add(time.Duration(c.Lock.TTLMin) * time.Minute)
	st.StaleAt = clock.Format(staleAt)
	st.Stale = now.After(staleAt)
	return st
}

func lockHeld(c *Change, st LockStatus) error {
	return aerr.New(aerr.StateLockHeld, fmt.Sprintf(
		"%s is locked by session %s since %s (last heartbeat %s, stale after %s); this session has read-only access",
		c.ID, st.Session, st.Since, st.HeartbeatAt, st.StaleAt)).
		WithNextAction("wait for the other session to finish, or once the lock is stale run /athel:resume --take-over")
}

// claim makes session the writer of c, refreshing its own lock. It fails while another
// session holds a live lock; a stale foreign lock still needs an explicit take-over.
func (s *Store) claim(c *Change, session string, now time.Time) error {
	st := Status(c, now)
	if st.Held && st.Session != session {
		return lockHeld(c, st)
	}
	stamp := clock.Format(now)
	if !st.Held {
		c.Lock = &Lock{Session: session, Since: stamp, TTLMin: s.ttl()}
	}
	c.Lock.HeartbeatAt = stamp
	return nil
}

func (s *Store) ttl() int {
	if s.TTLMin > 0 {
		return s.TTLMin
	}
	return DefaultTTLMin
}

// AcquireOptions controls AcquireLock.
type AcquireOptions struct {
	Takeover bool
	Reason   string
}

// AcquireLock makes session the single writer of change id. A live foreign lock is never
// broken. A stale one is broken only with Takeover and a reason, and the take-over is logged.
func (s *Store) AcquireLock(id, session string, opts AcquireOptions) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	var previous LockStatus
	c, err := s.Update(id, func(c *Change) error {
		if IsTerminal(c.State) {
			return aerr.New(aerr.StateInvalid, fmt.Sprintf("%s is %s; it can be read but not locked", c.ID, c.State))
		}
		now := s.Clock.Now()
		previous = Status(c, now)
		if !previous.Held || previous.Session == session {
			return s.claim(c, session, now)
		}
		if !previous.Stale {
			return lockHeld(c, previous)
		}
		if !opts.Takeover {
			return aerr.New(aerr.StateLockHeld, fmt.Sprintf("%s has a stale lock from session %s (last heartbeat %s)",
				c.ID, previous.Session, previous.HeartbeatAt)).
				WithNextAction("take it over explicitly: --takeover --reason \"…\"")
		}
		if strings.TrimSpace(opts.Reason) == "" {
			return aerr.New(aerr.StateInvalid, "a take-over needs a reason")
		}
		c.Lock = nil
		return s.claim(c, session, now)
	})
	if err != nil {
		return nil, err
	}
	if previous.Held && previous.Session != session {
		reason := fmt.Sprintf("took over stale lock of session %s (last heartbeat %s): %s",
			previous.Session, previous.HeartbeatAt, opts.Reason)
		if err := s.logEvent(&Event{Kind: EventLockTakeover, Change: c.ID, Session: &session, Reason: &reason}); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// RefreshLock renews session's heartbeat. To keep the PreToolUse hook from rewriting
// change.yaml on every tool call, it only writes when the heartbeat is older than the
// refresh interval (or force is set). It reports whether it wrote.
func (s *Store) RefreshLock(id, session string, force bool) (bool, error) {
	if err := CheckSession(session); err != nil {
		return false, err
	}
	c, err := s.Get(id)
	if err != nil {
		return false, err
	}
	now := s.Clock.Now()
	st := Status(c, now)
	if !st.Held {
		return false, aerr.New(aerr.StateInvalid, c.ID+" is not locked; acquire the lock first")
	}
	if st.Session != session {
		return false, lockHeld(c, st)
	}
	if hb, err := clock.Parse(st.HeartbeatAt); err == nil && !force && now.Sub(hb) < s.heartbeatEvery() {
		return false, nil
	}
	_, err = s.Update(id, func(c *Change) error { return s.claim(c, session, s.Clock.Now()) })
	return err == nil, err
}

func (s *Store) heartbeatEvery() time.Duration {
	if s.HeartbeatEvery > 0 {
		return s.HeartbeatEvery
	}
	return DefaultHeartbeatEvery
}

// ReleaseLock drops session's lock on id. Releasing a free lock is a no-op; only the holder
// may release a held one.
func (s *Store) ReleaseLock(id, session string) (*Change, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	return s.Update(id, func(c *Change) error {
		st := Status(c, s.Clock.Now())
		if st.Held && st.Session != session {
			return lockHeld(c, st)
		}
		c.Lock = nil
		return nil
	})
}

// ReleaseAllMine releases every lock held by session and returns the change IDs.
func (s *Store) ReleaseAllMine(session string) ([]string, error) {
	if err := CheckSession(session); err != nil {
		return nil, err
	}
	changes, err := s.List()
	if err != nil {
		return nil, err
	}
	released := []string{}
	for _, c := range changes {
		if c.Lock == nil || c.Lock.Session != session {
			continue
		}
		if _, err := s.ReleaseLock(c.ID, session); err != nil {
			return released, err
		}
		released = append(released, c.ID)
	}
	return released, nil
}
