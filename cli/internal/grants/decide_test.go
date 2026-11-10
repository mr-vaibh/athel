package grants

import (
	"strings"
	"testing"
)

func TestParseDecidePrompt(t *testing.T) {
	tests := []struct {
		prompt, change, words string
		ok                    bool
	}{
		{"/athel:decide CHG-001 D-1=b", "CHG-001", "D-1=b", true},
		{"  /athel:decide CHG-042 D-2=a D-1=b skip=D-3  ", "CHG-042", "D-1=b D-2=a skip=D-3", true},
		{"/athel:decide CHG-001 D-1='only E.164, no local formats'", "CHG-001", "D-1='only E.164, no local formats'", true},
		{"/athel:decide CHG-001", "", "", false},                   // no answers
		{"/athel:decide CHG-001 D-1=e", "", "", false},             // not an option key
		{"/athel:decide CHG-001 D-1=b please", "", "", false},      // a stray word
		{"/athel:decide CHG-001 D-1='it's'", "", "", false},        // a quote inside the text
		{"/athel:decide CHG-001 D-1='$(rm -rf /)'", "", "", false}, // shell expansion characters
		{"/athel:approve CHG-001", "", "", false},
		{"please /athel:decide CHG-001 D-1=b", "", "", false},
	}
	for _, tt := range tests {
		change, answers, ok := ParseDecidePrompt(tt.prompt)
		var words []string
		for _, a := range answers {
			words = append(words, a.word())
		}
		if ok != tt.ok || change != tt.change || strings.Join(words, " ") != tt.words {
			t.Errorf("%q: %q %q %v", tt.prompt, change, words, ok)
		}
	}
}

func TestDecideKeyAndCommand(t *testing.T) {
	_, a, _ := ParseDecidePrompt("/athel:decide CHG-001 D-2=a D-1='E.164 only' skip=D-3")
	_, b, _ := ParseDecidePrompt("/athel:decide CHG-001 skip=D-3 D-1='E.164 only' D-2=a")
	if DecideKey("CHG-001", a) != DecideKey("CHG-001", b) || !strings.HasPrefix(DecideKey("CHG-001", a), "decide:") {
		t.Error("the same answers in any order give the same key")
	}
	_, c, _ := ParseDecidePrompt("/athel:decide CHG-001 D-2=b D-1='E.164 only' skip=D-3")
	if DecideKey("CHG-001", a) == DecideKey("CHG-001", c) || DecideKey("CHG-001", a) == DecideKey("CHG-002", a) {
		t.Error("another answer or change gives another key")
	}
	cmd := DecideCommand("CHG-001", a)
	if cmd != `athel decide answer CHG-001 'D-1='"'"'E.164 only'"'"'' 'D-2=a' 'skip=D-3'` {
		t.Errorf("command: %s", cmd)
	}
	// The CLI receives the words with the shell's quoting removed, and must read the same answers.
	back, ok := ParseAnswers([]string{"D-1='E.164 only'", "D-2=a", "skip=D-3"})
	if !ok || DecideKey("CHG-001", back) != DecideKey("CHG-001", a) {
		t.Errorf("CLI words: %v %v", back, ok)
	}
}
