package errors

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestCodeFamilies(t *testing.T) {
	tests := []struct {
		code Code
		want Family
	}{
		{StateIllegalTransition, FamilyState},
		{PolicyHumanRequired, FamilyPolicy},
		{"E-AUTH-EXPIRED", FamilyAuth},
		{"E-TOOL-TIMEOUT", FamilyTool},
		{"E-GATE-RED", FamilyGate},
		{"E-BUDGET-EXCEEDED", FamilyBudget},
		{"E-CONFLICT-FOREIGN-PR", FamilyConflict},
		{"E-INTEGRITY-HASH", FamilyIntegrity},
		{"E-STATE", ""},
		{"E-STATEX-FOO", ""},
		{"E-UNKNOWN-FOO", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := tt.code.Family(); got != tt.want {
				t.Errorf("Family(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestEveryFamilyHasDefaultNextActionAndExitCode(t *testing.T) {
	for _, f := range families {
		if defaultNextAction[f] == "" {
			t.Errorf("family %s has no default next action", f)
		}
		if _, ok := exitByFamily[f]; !ok {
			t.Errorf("family %s has no exit code", f)
		}
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"untyped", stderrors.New("boom"), ExitInternal},
		{"unknown family", New("E-WHATEVER-X", "x"), ExitInternal},
		{"policy", New(PolicyHumanRequired, "x"), ExitPolicy},
		{"auth", New("E-AUTH-EXPIRED", "x"), ExitAuth},
		{"tool", New("E-TOOL-TIMEOUT", "x"), ExitTool},
		{"state", New(StateIllegalTransition, "x"), ExitState},
		{"gate", New("E-GATE-RED", "x"), ExitGate},
		{"budget", New("E-BUDGET-EXCEEDED", "x"), ExitBudget},
		{"conflict", New("E-CONFLICT-FOREIGN", "x"), ExitConflict},
		{"integrity", New("E-INTEGRITY-HASH", "x"), ExitIntegrity},
		{"wrapped by fmt", fmt.Errorf("context: %w", New(StateLockHeld, "x")), ExitState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWrapKeepsCodeAndCause(t *testing.T) {
	err := Wrap(StateIO, "write change.yaml", fs.ErrPermission)
	if !stderrors.Is(err, fs.ErrPermission) {
		t.Error("errors.Is did not find the cause")
	}
	if !HasCode(err, StateIO) {
		t.Error("HasCode did not find the code")
	}
	if HasCode(err, StateSchema) {
		t.Error("HasCode found a code that is not in the chain")
	}
	if HasCode(nil, StateIO) {
		t.Error("HasCode(nil) = true")
	}
	outer := fmt.Errorf("outer: %w", err)
	got, ok := As(outer)
	if !ok || got.Code != StateIO {
		t.Fatalf("As(outer) = %v, %v", got, ok)
	}
	if want := "E-STATE-IO: write change.yaml: permission denied"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if New(StateIO, "x").Error() != "E-STATE-IO: x" {
		t.Errorf("Error() without cause = %q", New(StateIO, "x").Error())
	}
	if _, ok := As(stderrors.New("plain")); ok {
		t.Error("As found an *Error in a plain error")
	}
}

func TestHasCodeFollowsJoin(t *testing.T) {
	joined := stderrors.Join(New(StateIllegalTransition, "a"), fmt.Errorf("audit: %w", New(StateIO, "b")))
	for _, code := range []Code{StateIllegalTransition, StateIO} {
		if !HasCode(joined, code) {
			t.Errorf("HasCode(joined, %s) = false", code)
		}
	}
	if HasCode(joined, StateSchema) {
		t.Error("HasCode found a code that is not in the tree")
	}
	if ExitCode(joined) != ExitState {
		t.Errorf("ExitCode(joined) = %d", ExitCode(joined))
	}
}

func TestWithStepAndNextAction(t *testing.T) {
	e := New(StateIllegalTransition, "x")
	if e.NextAction != defaultNextAction[FamilyState] {
		t.Errorf("default next action = %q", e.NextAction)
	}
	e = e.WithStep("build").WithNextAction("ask the human")
	if e.Step != "build" || e.NextAction != "ask the human" {
		t.Errorf("got step %q next %q", e.Step, e.NextAction)
	}
}

func TestErrorJSONShape(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "full",
			err:  Wrap(StateIO, "write", fs.ErrPermission).WithStep("build"),
			want: `{"code":"E-STATE-IO","message":"write","step":"build","next_action":"stop the step; never auto-fix state","cause":"permission denied"}`,
		},
		{
			name: "minimal",
			err:  &Error{Code: "E-TOOL-X", Message: "m"},
			want: `{"code":"E-TOOL-X","message":"m"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.err)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("json = %s\nwant   %s", got, tt.want)
			}
		})
	}
}
