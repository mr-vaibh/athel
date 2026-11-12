package blast

import (
	"strings"
	"testing"
)

func TestCheckPlanned(t *testing.T) {
	r := &Report{
		Direct:      []Direct{{Item: "PUT /v1/users/{id}/phone", Evidence: []Evidence{{Kind: "spec", Ref: "REQ-1"}}}},
		Data:        []Migration{{Table: "users", Evidence: []Evidence{{Kind: "spec", Ref: "REQ-2"}, {Kind: "code", Ref: "m.sql"}}}},
		NotAffected: []NotAffected{{}},
	}
	reqs := []string{"REQ-1", "REQ-2"}
	if p := CheckPlanned(r, reqs); len(p) != 0 {
		t.Fatalf("problems: %v", p)
	}
	r.Direct[0].Evidence = []Evidence{{Kind: "spec", Ref: "REQ-9"}}
	r.Data[0].Evidence = []Evidence{{Kind: "code", Ref: "m.sql"}}
	r.NotAffected = nil
	got := strings.Join(CheckPlanned(r, reqs), "; ")
	for _, want := range []string{"changed item PUT /v1/users/{id}/phone cites no requirement", "data change on users cites no requirement", "not_affected is empty"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	if p := CheckPlanned(r, nil); len(p) != 1 || !strings.Contains(p[0], "has none yet") {
		t.Errorf("no spec: %v", p)
	}
}
