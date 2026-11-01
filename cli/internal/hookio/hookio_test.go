package hookio

import "testing"

func TestEventsMatchDoc02HooksTable(t *testing.T) {
	want := []Event{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure",
		"SubagentStop", "Stop", "PreCompact", "SessionEnd"}
	if len(Events) != len(want) {
		t.Fatalf("Events = %v", Events)
	}
	for i := range want {
		if Events[i] != want[i] {
			t.Errorf("Events[%d] = %s, want %s", i, Events[i], want[i])
		}
	}
}
