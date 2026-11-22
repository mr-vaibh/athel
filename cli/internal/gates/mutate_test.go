package gates

import (
	"strings"
	"testing"
)

const mutSrc = `package src

func Check(a, b int, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	ok := a == b && a < 10
	return ok || true, nil
}

func Untouched(a int) bool { return a > 1 }
`

func TestMutants(t *testing.T) {
	ms, err := Mutants("src/check.go", mutSrc, []int{4, 5, 6, 7, 8})
	if err != nil {
		t.Fatal(err)
	}
	var descs []string
	for _, m := range ms {
		descs = append(descs, m.Desc)
		if m.File != "src/check.go" || m.Src == mutSrc {
			t.Errorf("a mutant changes the file: %+v", m.Desc)
		}
	}
	got := strings.Join(descs, "; ")
	for _, want := range []string{
		"line 4: != to ==", "line 4: drop the err != nil return", "line 7: == to !=", "line 7: && to ||",
		"line 7: < to <=", "line 8: || to &&", "line 8: true to false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing mutant %q in %s", want, got)
		}
	}
	if strings.Contains(got, "line 11") {
		t.Errorf("only changed lines are mutated: %s", got)
	}
	for _, m := range ms {
		if m.Desc == "line 4: drop the err != nil return" && strings.Contains(m.Src, "return false, err") {
			t.Errorf("the return is dropped:\n%s", m.Src)
		}
	}
	if _, err := Mutants("bad.go", "package", []int{1}); err == nil {
		t.Error("unparseable Go must fail")
	}
}

func TestMutantsCapped(t *testing.T) {
	src := "package src\n\nfunc F(a int) bool {\n" + strings.Repeat("\t_ = a == 1\n", 80) + "\treturn true\n}\n"
	var lines []int
	for i := 1; i <= 90; i++ {
		lines = append(lines, i)
	}
	ms, _ := Mutants("f.go", src, lines)
	if len(Cap(ms)) != MaxMutants {
		t.Errorf("at most %d mutants: %d", MaxMutants, len(Cap(ms)))
	}
}

func TestScore(t *testing.T) {
	if s := Score(7, 3); s != 70 {
		t.Errorf("%v", s)
	}
	if s := Score(0, 0); s != 100 {
		t.Errorf("no mutants: %v", s)
	}
}

func TestMutantsOnlyDropErrorReturns(t *testing.T) {
	src := "package src\n\nfunc F(ok bool, err error) {\n\tif ok {\n\t\treturn\n\t}\n\tif err == nil {\n\t\treturn\n\t}\n}\n"
	ms, err := Mutants("f.go", src, []int{4, 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if strings.Contains(m.Desc, "drop") {
			t.Errorf("only `if err != nil` returns are dropped: %s", m.Desc)
		}
	}
	if len(Cap(ms)) != len(ms) {
		t.Error("a short list is kept whole")
	}
}
