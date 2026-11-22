package grants

import (
	"strings"
	"testing"
)

func TestParseTriagePrompt(t *testing.T) {
	change, choices, ok := ParseTriagePrompt("/athel:triage CHG-001 SF-1=fix_pr SF-3=ticket SF-2=ignore")
	if !ok || change != "CHG-001" || strings.Join(choices, " ") != "SF-1=fix_pr SF-2=ignore SF-3=ticket" {
		t.Fatalf("%q %v %v", change, choices, ok)
	}
	if _, all, ok := ParseTriagePrompt("/athel:triage CHG-001 all=ticket"); !ok || strings.Join(all, " ") != "all=ticket" {
		t.Errorf("one answer for all: %v %v", all, ok)
	}
	for _, bad := range []string{
		"/athel:triage CHG-001", "/athel:triage CHG-001 SF-1=later", "/athel:triage CHG-001 SF-1=ticket SF-1=ignore",
		"/athel:triage CHG-001 all=ticket SF-1=ignore", "/athel:triage CHG-001 SF-1=ticket; rm -rf ~", "triage CHG-001 SF-1=ticket",
	} {
		if _, _, ok := ParseTriagePrompt(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
	if TriageKey("CHG-001", []string{"SF-1=ticket"}) == TriageKey("CHG-001", []string{"SF-1=ignore"}) {
		t.Error("the key binds the choices")
	}
	if got := TriageCommand("CHG-001", []string{"SF-1=fix_pr", "SF-2=ticket"}); got != "athel finding triage CHG-001 SF-1=fix_pr SF-2=ticket" {
		t.Errorf("%q", got)
	}
}
