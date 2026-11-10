package grants

import (
	"strings"
	"testing"
)

func TestParseRootsPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   []string
	}{
		{"/athel:roots ~/Developer", []string{"~/Developer"}},
		{"  /athel:roots ~/snabbit/ /Users/dev/work  ~/Developer ", []string{"/Users/dev/work", "~/Developer", "~/snabbit"}},
		{"/athel:roots ~/a ~/a/", []string{"~/a"}},
		{"/athel:roots", nil},
		{"/athel:roots relative/path", nil},
		{"/athel:roots ~/a then index", nil},
		{"please run /athel:roots ~/a", nil},
		{"/athel:roots ~/a\n/athel:roots ~/b", nil},
		{"/athel:roots /", nil},
		{"/athel:approve CHG-001", nil},
		{"/athel:roots ~/it's", nil},
		{"/athel:roots ~/$HOME", nil},
	}
	for _, tt := range tests {
		got, ok := ParseRootsPrompt(tt.prompt)
		if ok != (tt.want != nil) || strings.Join(got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("ParseRootsPrompt(%q) = %v %v, want %v", tt.prompt, got, ok, tt.want)
		}
	}
}

func TestRootsKey(t *testing.T) {
	a := RootsKey([]string{"~/b", "~/a/"})
	if !strings.HasPrefix(a, "ROOTS-") || len(a) != len("ROOTS-")+12 {
		t.Fatalf("key = %q", a)
	}
	if RootsKey([]string{"~/a", "~/b"}) != a {
		t.Error("order and trailing slashes must not change the key")
	}
	if RootsKey([]string{"~/a"}) == a {
		t.Error("different roots share a key")
	}
	if got, ok := ParseRootsPrompt("/athel:roots ~/a ~/b"); !ok || RootsKey(got) != a {
		t.Error("the prompt's roots and the command's roots must give the same key")
	}
}

func TestRootsCommand(t *testing.T) {
	if got := RootsCommand([]string{"~/b/", "/Users/dev/a"}); got != "athel repos roots set '/Users/dev/a' '~/b'" {
		t.Errorf("command = %q", got)
	}
}
