package state

import (
	stderrors "errors"
	"path/filepath"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/eventlog"
)

// EventSchemaID is the schema of one line of audit/state-events.jsonl.
const EventSchemaID = "athel/state-event@1"

// Kinds of state events.
const (
	EventTransition        = "transition"
	EventIllegalTransition = "illegal_transition"
	EventHumanRequired     = "human_required"
	EventStepEnter         = "step_enter"
	EventStepExit          = "step_exit"
	EventLockTakeover      = "lock_takeover"
)

// Event is one line of audit/state-events.jsonl. Field names follow the doc 04 audit
// record where they overlap.
type Event struct {
	Schema    string  `json:"schema"`
	Seq       int     `json:"seq"`
	TS        string  `json:"ts"`
	Kind      string  `json:"kind"`
	Change    string  `json:"change"`
	Session   *string `json:"session"`
	Step      *string `json:"step"`
	From      *string `json:"from"`
	To        *string `json:"to"`
	By        *string `json:"by"`
	Reason    *string `json:"reason"`
	ErrorCode *string `json:"error_code"`
}

// Stamp sets the sequence number and timestamp (eventlog.Record).
func (e *Event) Stamp(seq int, ts string) { e.Seq, e.TS = seq, ts }

// StateEventsFile is the M0 log of state events.
func (w Workspace) StateEventsFile() string { return filepath.Join(w.AuditDir(), "state-events.jsonl") }

func (s *Store) logEvent(e *Event) error {
	e.Schema = EventSchemaID
	l := &eventlog.Log{Path: s.WS.StateEventsFile(), Clock: s.Clock, LockTimeout: s.LockTimeout}
	return l.Append(e)
}

func mustCode(err error) aerr.Code {
	if e, ok := aerr.As(err); ok {
		return e.Code
	}
	return ""
}

// errorsJoin keeps the primary error first so its code decides the exit code.
func errorsJoin(primary, secondary error) error { return stderrors.Join(primary, secondary) }
