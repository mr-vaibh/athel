package ids

import (
	"testing"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func TestParseChangeID(t *testing.T) {
	tests := []struct {
		in      string
		want    ChangeID
		wantErr bool
	}{
		{in: "CHG-001", want: ChangeID{Number: 1}},
		{in: "CHG-042", want: ChangeID{Number: 42}},
		{in: "CHG-999", want: ChangeID{Number: 999}},
		{in: "CHG-1000", want: ChangeID{Number: 1000}},
		{in: "CHG-042-F3", want: ChangeID{Number: 42, Child: 3}},
		{in: "CHG-042-F12", want: ChangeID{Number: 42, Child: 12}},
		{in: "CHG-000", wantErr: true},
		{in: "CHG-42", wantErr: true},
		{in: "CHG-0042", wantErr: true},
		{in: "CHG-01000", wantErr: true},
		{in: "CHG-042-F0", wantErr: true},
		{in: "CHG-042-F", wantErr: true},
		{in: "chg-042", wantErr: true},
		{in: "CHG-042 ", wantErr: true},
		{in: "CHG-99999999999999999999", wantErr: true},
		{in: "", wantErr: true},
		{in: "../CHG-001", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseChange(tt.in)
			if tt.wantErr {
				if !aerr.HasCode(err, aerr.StateInvalid) {
					t.Fatalf("err = %v, want E-STATE-INVALID", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if got.String() != tt.in {
				t.Errorf("String() = %q, want %q", got.String(), tt.in)
			}
		})
	}
}

func TestParseChangeChildOverflow(t *testing.T) {
	if _, err := ParseChange("CHG-042-F99999999999999999999"); !aerr.HasCode(err, aerr.StateInvalid) {
		t.Errorf("err = %v, want E-STATE-INVALID", err)
	}
}

func TestNextChangeID(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		want     string
	}{
		{"empty workspace", nil, "CHG-001"},
		{"sequential", []string{"CHG-001", "CHG-002"}, "CHG-003"},
		{"gap keeps max", []string{"CHG-001", "CHG-007"}, "CHG-008"},
		{"unsorted", []string{"CHG-010", "CHG-002"}, "CHG-011"},
		{"ignores children", []string{"CHG-003", "CHG-003-F9"}, "CHG-004"},
		{"ignores junk", []string{".lock", "notes", "CHG-x", "CHG-005"}, "CHG-006"},
		{"rolls past 999", []string{"CHG-999"}, "CHG-1000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextChange(tt.existing); got != tt.want {
				t.Errorf("NextChange = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFactAndAskIDs(t *testing.T) {
	tests := []struct {
		seq     Seq
		in      string
		want    int
		wantErr bool
	}{
		{seq: Fact, in: "F-0001", want: 1},
		{seq: Fact, in: "F-0193", want: 193},
		{seq: Fact, in: "F-9999", want: 9999},
		{seq: Fact, in: "F-10000", want: 10000},
		{seq: Fact, in: "F-0000", wantErr: true},
		{seq: Fact, in: "F-193", wantErr: true},
		{seq: Fact, in: "F-00193", wantErr: true},
		{seq: Fact, in: "F-010000", wantErr: true},
		{seq: Fact, in: "f-0001", wantErr: true},
		{seq: Fact, in: "F-0001 ", wantErr: true},
		{seq: Fact, in: "F-99999999999999999999", wantErr: true},
		{seq: Fact, in: "ASK-001", wantErr: true},
		{seq: Ask, in: "ASK-001", want: 1},
		{seq: Ask, in: "ASK-1000", want: 1000},
		{seq: Ask, in: "ASK-01", wantErr: true},
		{seq: Ask, in: "ASK-000", wantErr: true},
		{seq: Ask, in: "CHG-001", wantErr: true},
		{seq: Ask, in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.seq.Prefix+tt.in, func(t *testing.T) {
			got, err := tt.seq.Parse(tt.in)
			if tt.wantErr {
				if !aerr.HasCode(err, aerr.StateInvalid) {
					t.Fatalf("err = %v, want E-STATE-INVALID", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Parse = %d, %v; want %d", got, err, tt.want)
			}
			if tt.seq.Format(got) != tt.in {
				t.Errorf("Format(%d) = %q, want %q", got, tt.seq.Format(got), tt.in)
			}
		})
	}
}

func TestNextSeqID(t *testing.T) {
	tests := []struct {
		name     string
		seq      Seq
		existing []string
		want     string
	}{
		{"first fact", Fact, nil, "F-0001"},
		{"after highest fact", Fact, []string{"F-0002", "F-0193", "F-0007"}, "F-0194"},
		{"ignores junk and other kinds", Fact, []string{"F-12", "ASK-009", "x", "F-0003"}, "F-0004"},
		{"rolls past 9999", Fact, []string{"F-9999"}, "F-10000"},
		{"first ask", Ask, []string{".lock"}, "ASK-001"},
		{"after highest ask", Ask, []string{"ASK-001", "ASK-010"}, "ASK-011"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seq.Next(tt.existing); got != tt.want {
				t.Errorf("Next = %q, want %q", got, tt.want)
			}
		})
	}
}
