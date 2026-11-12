package prd

import (
	"slices"
	"strings"
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const v1 = `Intro line that says nothing.
The service must start.

# Phone number

Customers must be able to change their phone number. This is nice.
- Numbers are stored in E.164
- Changes are published as events

` + "```" + `
must not be indexed: example code
` + "```" + `

## Limits
Updates should finish within 200 ms.
`

func ids(x Index) []string {
	out := []string{}
	for _, it := range x.Items {
		out = append(out, it.ID+" "+it.Kind+" "+it.Text)
	}
	return out
}

func TestBuildAssignsIDs(t *testing.T) {
	got := ids(Build("CHG-001", "v1", "sha256:x", []byte(v1), nil))
	want := []string{
		"PRD-0.1 statement The service must start.",
		"PRD-1 heading Phone number",
		"PRD-1.1 statement Customers must be able to change their phone number.",
		"PRD-1.2 statement Numbers are stored in E.164",
		"PRD-1.3 statement Changes are published as events",
		"PRD-2 heading Limits",
		"PRD-2.1 statement Updates should finish within 200 ms.",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ids:\n%v\nwant\n%v", got, want)
	}
}

func TestBuildIsStableAcrossVersions(t *testing.T) {
	a := Build("CHG-001", "v1", "sha256:x", []byte(v1), nil)
	if again := Build("CHG-001", "v1", "sha256:x", []byte(v1), nil); !slices.Equal(ids(a), ids(again)) {
		t.Fatal("the same text must get the same IDs")
	}
	v2 := "# Scope\n- Only signed-in customers\n\n" + v1 + "- Numbers are stored in E.164\n"
	v2 = replace(v2, "Changes are published as events", "Changes are published as v2 events")
	b := Build("CHG-001", "v2", "sha256:y", []byte(v2), &a)
	got := ids(b)
	must := func(s string) {
		t.Helper()
		if !slices.Contains(got, s) {
			t.Errorf("missing %q in\n%v", s, got)
		}
	}
	must("PRD-3 heading Scope")
	must("PRD-3.1 statement Only signed-in customers")
	must("PRD-1.1 statement Customers must be able to change their phone number.")
	must("PRD-1.4 statement Changes are published as v2 events")
	must("PRD-2.1 statement Updates should finish within 200 ms.")
	must("PRD-2.2 statement Numbers are stored in E.164") // the same text twice gets a second ID
	d := Compare(a, b)
	if len(d.Removed) != 1 || d.Removed[0].ID != "PRD-1.3" || len(d.Added) != 4 {
		t.Errorf("diff: %+v", d)
	}
}

func replace(s, old, new string) string {
	i := index(s, old)
	return s[:i] + new + s[i+len(old):]
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestHas(t *testing.T) {
	x := Build("CHG-001", "v1", "sha256:x", []byte(v1), nil)
	if !x.Has("PRD-1.2") || x.Has("PRD-9") {
		t.Error("Has")
	}
}

func TestScan(t *testing.T) {
	x := Build("CHG-001", "v1", "sha256:x", []byte("# Speed\n- The page must be fast, secure etc.\n- It must not break breakfast or supported flows.\n- Answer within 200 ms, 99.9% of the time, and retry 3 times.\n"), nil)
	amb, lim := Scan(x)
	phrases := func(fs []Finding) []string {
		out := []string{}
		for _, f := range fs {
			out = append(out, f.PRD+" "+f.Phrase)
		}
		return out
	}
	if got := phrases(amb); !slices.Equal(got, []string{"PRD-1.1 fast", "PRD-1.1 secure", "PRD-1.1 etc."}) {
		t.Errorf("ambiguous: %v", got)
	}
	if got := phrases(lim); !slices.Equal(got, []string{"PRD-1.3 200 ms", "PRD-1.3 99.9%", "PRD-1.3 3 times"}) {
		t.Errorf("limits: %v", got)
	}
}

func TestEncodeDecodeIndex(t *testing.T) {
	x := Build("CHG-001", "v1", "sha256:"+strings.Repeat("a", 64), []byte(v1), nil)
	reg := schema.NewRegistry()
	got, err := DecodeIndex(reg, Encode(x))
	if err != nil || !slices.Equal(ids(*got), ids(x)) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	for name, doc := range map[string]string{"unknown field": string(Encode(x)) + "extra: 1\n", "bad sha": strings.Replace(string(Encode(x)), "sha256:aaaa", "md5:aaaa", 1)} {
		if _, err := DecodeIndex(reg, []byte(doc)); !aerr.HasCode(err, aerr.StateSchema) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestSentencesEndAtPunctuationAndSpace: a dot inside a token (E.164, v1.2, 2.5) does not end a
// sentence; the first M3 smoke run indexed "stored in E." as a whole requirement.
func TestSentencesEndAtPunctuationAndSpace(t *testing.T) {
	for _, tc := range []struct{ line, want string }{
		{"Numbers must be stored in E.164. Nice.", "Numbers must be stored in E.164."},
		{"The API must answer within 2.5 s on v1.2!", "The API must answer within 2.5 s on v1.2!"},
		{"It must work", "It must work"},
		{"Fine. It must stop?? Done.", "It must stop??"},
	} {
		got := parse([]byte(tc.line))
		if len(got) != 1 || got[0].text != tc.want {
			t.Errorf("%q: %+v, want %q", tc.line, got, tc.want)
		}
	}
}
