package state

import (
	stderrors "errors"

	"github.com/mr-vaibh/athel/cli/internal/audit"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/grants"
)

// Event kinds this package writes to the audit log.
const (
	EventTransition        = audit.KindTransition
	EventIllegalTransition = audit.KindIllegalTransition
	EventHumanRequired     = audit.KindHumanRequired
	EventStepEnter         = audit.KindStepEnter
	EventStepExit          = audit.KindStepExit
	EventLockTakeover      = audit.KindLockTakeover
	EventGrantUsed         = audit.KindGrantUsed
)

// Event is a state event before it is stamped and chained into the audit log.
type Event struct {
	Kind      string
	Change    string
	Session   *string
	Step      *string
	From      *string
	To        *string
	By        *string
	Reason    *string
	ErrorCode *string
}

// Grants is the workspace's book of human-origin grants.
func (s *Store) Grants() grants.Book {
	return grants.Book{Dir: s.WS.StateDir(), Clock: s.Clock, LockTimeout: s.LockTimeout}
}

// AuditLog is the workspace's hash-chained audit log.
func (s *Store) AuditLog() *audit.Log {
	return &audit.Log{Dir: s.WS.AuditDir(), Clock: s.Clock, LockTimeout: s.LockTimeout}
}

func (s *Store) logEvent(e *Event) error {
	result := audit.ResultOK
	if e.ErrorCode != nil {
		result = audit.ResultError
	}
	_, err := s.AuditLog().Append(audit.Record{
		Kind: e.Kind, Change: audit.Str(e.Change), Session: e.Session, Step: e.Step,
		From: e.From, To: e.To, By: e.By, Reason: e.Reason, Code: e.ErrorCode, Result: result,
	})
	return err
}

func mustCode(err error) aerr.Code {
	if e, ok := aerr.As(err); ok {
		return e.Code
	}
	return ""
}

// errorsJoin keeps the primary error first so its code decides the exit code.
func errorsJoin(primary, secondary error) error { return stderrors.Join(primary, secondary) }
