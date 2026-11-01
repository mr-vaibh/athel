// Package errors defines Athel's typed errors. Every error the CLI reports carries one code
// from the doc 02 error taxonomy (E-POLICY, E-AUTH, E-TOOL, E-STATE, E-GATE, E-BUDGET,
// E-CONFLICT, E-INTEGRITY), a message, the step it happened in and the next action.
package errors

import (
	"encoding/json"
	stderrors "errors"
	"strings"
)

// Code is a full error code such as "E-STATE-ILLEGAL-TRANSITION". Its family is the prefix.
type Code string

// Family is one error class of the doc 02 taxonomy.
type Family string

// The eight families of the doc 02 error taxonomy.
const (
	FamilyPolicy    Family = "E-POLICY"
	FamilyAuth      Family = "E-AUTH"
	FamilyTool      Family = "E-TOOL"
	FamilyState     Family = "E-STATE"
	FamilyGate      Family = "E-GATE"
	FamilyBudget    Family = "E-BUDGET"
	FamilyConflict  Family = "E-CONFLICT"
	FamilyIntegrity Family = "E-INTEGRITY"
)

// Codes used by M0. Later milestones add codes; existing codes never change meaning.
const (
	StateIllegalTransition Code = "E-STATE-ILLEGAL-TRANSITION"
	StateSchema            Code = "E-STATE-SCHEMA"
	StateNotFound          Code = "E-STATE-NOT-FOUND"
	StateNotInitialised    Code = "E-STATE-NOT-INITIALISED"
	StateLockHeld          Code = "E-STATE-LOCK-HELD"
	StateLockTimeout       Code = "E-STATE-LOCK-TIMEOUT"
	StateNoSession         Code = "E-STATE-NO-SESSION"
	StateInvalid           Code = "E-STATE-INVALID"
	StateIO                Code = "E-STATE-IO"
	PolicyHumanRequired    Code = "E-POLICY-HUMAN-REQUIRED"
	PolicyConfigLoosens    Code = "E-POLICY-CONFIG-LOOSENS"
	PolicyConfigInvalid    Code = "E-POLICY-CONFIG-INVALID"
)

// families is ordered so the longest prefix that could be ambiguous is irrelevant: no family
// name is a prefix of another.
var families = [...]Family{
	FamilyPolicy, FamilyAuth, FamilyTool, FamilyState,
	FamilyGate, FamilyBudget, FamilyConflict, FamilyIntegrity,
}

// Family returns the taxonomy family of the code, or "" for a code outside the taxonomy.
func (c Code) Family() Family {
	for _, f := range families {
		if strings.HasPrefix(string(c), string(f)+"-") {
			return f
		}
	}
	return ""
}

// defaultNextAction is the "default next action" column of the doc 02 taxonomy.
var defaultNextAction = map[Family]string{
	FamilyPolicy:    "record; raise a decision card only if the plan requires the action",
	FamilyAuth:      "block; tell the human which login to refresh",
	FamilyTool:      "retry per policy, then block",
	FamilyState:     "stop the step; never auto-fix state",
	FamilyGate:      "return to the build loop",
	FamilyBudget:    "block with an extend-or-stop decision card",
	FamilyConflict:  "block; never proceed over someone else's work",
	FamilyIntegrity: "stop everything for this change; human review",
}

// Error is Athel's typed error.
type Error struct {
	Code       Code
	Message    string
	Step       string
	NextAction string
	cause      error
}

// New returns an error with the family's default next action.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message, NextAction: defaultNextAction[code.Family()]}
}

// Wrap returns an error that keeps cause reachable through errors.Is and errors.As.
func Wrap(code Code, message string, cause error) *Error {
	e := New(code, message)
	e.cause = cause
	return e
}

// WithStep records the workflow step the error happened in.
func (e *Error) WithStep(step string) *Error {
	e.Step = step
	return e
}

// WithNextAction replaces the family's default next action with a specific one.
func (e *Error) WithNextAction(action string) *Error {
	e.NextAction = action
	return e
}

func (e *Error) Error() string {
	msg := string(e.Code) + ": " + e.Message
	if e.cause != nil {
		msg += ": " + e.cause.Error()
	}
	return msg
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *Error) Unwrap() error { return e.cause }

// jsonError is the stable wire shape used in `--json` output.
type jsonError struct {
	Code       Code   `json:"code"`
	Message    string `json:"message"`
	Step       string `json:"step,omitempty"`
	NextAction string `json:"next_action,omitempty"`
	Cause      string `json:"cause,omitempty"`
}

// MarshalJSON renders the error in the documented `--json` error shape.
func (e *Error) MarshalJSON() ([]byte, error) {
	j := jsonError{Code: e.Code, Message: e.Message, Step: e.Step, NextAction: e.NextAction}
	if e.cause != nil {
		j.Cause = e.cause.Error()
	}
	return json.Marshal(j)
}

// As returns the first *Error in err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := stderrors.As(err, &e)
	return e, ok
}

// HasCode reports whether any *Error in err's chain has the given code.
func HasCode(err error, code Code) bool {
	for err != nil {
		if e, ok := err.(*Error); ok && e.Code == code { //nolint:errorlint // walking the chain manually to check every link
			return true
		}
		err = stderrors.Unwrap(err)
	}
	return false
}
