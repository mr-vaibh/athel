package reports

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

func isTest(p string) bool { return strings.HasSuffix(p, "_test.go") }

func finding() Finding {
	return Finding{Severity: "major", Location: "a.go:3", Rule: "errors.no-silent-catch", Finding: "x", Required: "y"}
}

func TestCheckBuild(t *testing.T) {
	cases := []struct {
		name string
		d    BuildDraft
		want string
	}{
		{"ok", BuildDraft{CommandsRun: []Command{{"go test ./...", Pass}}}, ""},
		{"no commands", BuildDraft{}, "commands_run"},
		{"bad result", BuildDraft{CommandsRun: []Command{{"go test ./...", "ok"}}}, "pass or fail"},
		{"empty command", BuildDraft{CommandsRun: []Command{{" ", Pass}}}, "pass or fail"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expect(t, CheckBuild(c.d), c.want) })
	}
}

func TestCheckQA(t *testing.T) {
	changed := []string{"a_test.go", "a.go"}
	ok := Case{Category: "negative", File: "a_test.go", Name: "TestA", Result: Pass}
	with := func(f func(*QADraft)) QADraft {
		d := QADraft{Cases: []Case{ok}, Suite: Command{"go test ./...", Pass}}
		f(&d)
		return d
	}
	cases := []struct {
		name string
		d    QADraft
		want string
	}{
		{"ok", with(func(*QADraft) {}), ""},
		{"no cases", with(func(d *QADraft) { d.Cases = nil }), "lists the cases"},
		{"category", with(func(d *QADraft) { d.Cases[0].Category = "happy" }), "category"},
		{"no name", with(func(d *QADraft) { d.Cases[0].Name = "" }), "test name"},
		{"result", with(func(d *QADraft) { d.Cases[0].Result = "skip" }), "test name"},
		{"product file", with(func(d *QADraft) { d.Cases[0].File = "a.go" }), "a.go is not a test file"},
		{"untouched test", with(func(d *QADraft) { d.Cases[0].File = "b_test.go" }), "not changed by this task"},
		{"suite", with(func(d *QADraft) { d.Suite.Result = "" }), "pass or fail"},
		{"failing case without finding", with(func(d *QADraft) {
			d.Cases = []Case{{Category: "boundary", File: "a_test.go", Name: "TestB", Result: Fail}}
		}), "file it under findings"},
		{"failing suite without finding", with(func(d *QADraft) { d.Suite.Result = Fail }), "file it under findings"},
		{"failing with finding", with(func(d *QADraft) { d.Suite.Result, d.Findings = Fail, []Finding{finding()} }), ""},
		{"bad finding", with(func(d *QADraft) { d.Findings = []Finding{{Severity: "major"}} }), "location, a rule"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expect(t, CheckQA(c.d, isTest, changed), c.want) })
	}
}

func TestCheckSecurity(t *testing.T) {
	cwe := finding()
	cwe.CWE = "CWE-79"
	cases := []struct {
		name string
		d    SecurityDraft
		want string
	}{
		{"ok", SecurityDraft{Checked: []string{"input"}, Findings: []Finding{cwe}}, ""},
		{"nothing checked", SecurityDraft{}, "what you checked"},
		{"no cwe", SecurityDraft{Checked: []string{"input"}, Findings: []Finding{finding()}}, "CWE"},
		{"severity", SecurityDraft{Checked: []string{"input"}, Findings: []Finding{{Severity: "high", CWE: "CWE-79"}}}, "severity"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { expect(t, CheckSecurity(c.d), c.want) })
	}
}

func TestNumber(t *testing.T) {
	got := Number("S", 2, []Finding{finding(), finding()})
	if got[0].ID != "S2-01" || got[1].ID != "S2-02" {
		t.Errorf("%+v", got)
	}
}

// TestTemplatesAreValidDrafts keeps each template a draft its check accepts (M3 lesson).
func TestTemplatesAreValidDrafts(t *testing.T) {
	var b BuildDraft
	var q QADraft
	var s SecurityDraft
	for _, x := range []struct {
		src []byte
		v   any
	}{{BuildTemplate, &b}, {QATemplate, &q}, {SecurityTemplate, &s}} {
		dec := yaml.NewDecoder(strings.NewReader(string(x.src)))
		dec.KnownFields(true)
		if err := dec.Decode(x.v); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{}
	for _, c := range q.Cases {
		files = append(files, c.File)
	}
	for _, err := range []error{CheckBuild(b), CheckQA(q, isTest, files), CheckSecurity(s)} {
		if err != nil {
			t.Error(err)
		}
	}
}

func expect(t *testing.T, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Fatal(err)
	case want != "" && (!aerr.HasCode(err, aerr.StateInvalid) || !strings.Contains(err.Error(), want)):
		t.Fatalf("want %q, got %v", want, err)
	}
}
