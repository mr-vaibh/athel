package gates

import (
	"strings"
	"testing"
)

func ids(gs []Gate) string {
	var out []string
	for _, g := range gs {
		out = append(out, g.ID)
	}
	return strings.Join(out, ",")
}

func TestDefaults(t *testing.T) {
	g := Defaults(Go, nil)
	if ids(g) != "format,lint,build,types,unit,ac-tests,contract,coverage,complexity" {
		t.Fatalf("go gates in doc 06 order: %s", ids(g))
	}
	for i, gate := range g {
		if (gate.Stage == Cheap) != (i < 6) || !gate.Available() {
			t.Errorf("%+v", gate)
		}
	}
	if !g[8].Builtin || len(g[8].Command) != 0 || !strings.Contains(strings.Join(g[7].Command, " "), "-coverprofile="+ProfilePlaceholder) {
		t.Errorf("coverage runs the tests with a profile; complexity is computed by the CLI: %+v", g[7:])
	}
	if g[0].Pass != NoOutput || g[1].Pass != NewFindings || g[4].Pass != Tests || g[1].Waivable != Human || g[2].Waivable != Never {
		t.Errorf("pass criteria and waivers: %+v", g)
	}
}

func TestNodeAndUnknownDefaults(t *testing.T) {
	node := Defaults(Node, map[string]bool{"lint": true, "test": true})
	for _, gate := range node {
		has := len(gate.Command) > 0
		if want := gate.ID == "lint" || gate.ID == "unit"; has != want || gate.Available() != (want || gate.Builtin) {
			t.Errorf("node %s has command %v; a gate without its package.json script is unavailable", gate.ID, gate.Command)
		}
	}
	if node[4].Command[2] != "--silent" || node[4].Command[3] != "test" {
		t.Errorf("node unit = %v", node[4].Command)
	}
	for _, gate := range Defaults("", nil) {
		if gate.Available() != (gate.ID == "ac-tests" || gate.ID == "contract") {
			t.Errorf("an unknown language has no default commands: %+v", gate)
		}
	}
}

func TestDetect(t *testing.T) {
	for files, want := range map[string]string{"go.mod,main.go": Go, "package.json": Node, "README.md": "", "go.mod,package.json": Go} {
		if got := Detect(strings.Split(files, ",")); got != want {
			t.Errorf("%s: %q, want %q", files, got, want)
		}
	}
}

func TestFindingsAgainstBaseline(t *testing.T) {
	base := "main.go:14:12: G114: Use of net/http serve function (gosec)\n1 issues:\n* gosec: 1\n"
	now := "main.go:20:12: G114: Use of net/http serve function (gosec)\napi/a.go:3:1: exported func X should have comment (revive)\n2 issues:\n"
	got := NewLines(base, now)
	if len(got) != 1 || !strings.Contains(got[0], "api/a.go") {
		t.Errorf("only the new finding counts, line moves do not: %q", got)
	}
	twice := "main.go:14:12: G114: Use of net/http serve function (gosec)\nmain.go:30:12: G114: Use of net/http serve function (gosec)\n"
	if got := NewLines(base, twice); len(got) != 1 {
		t.Errorf("a second identical finding is new: %q", got)
	}
}

func TestDisableComments(t *testing.T) {
	added := "x := 1\nfoo() //nolint:errcheck // fine\n/* eslint-disable */\n// eslint-disable-next-line no-console\nnolintAnything := 2\n"
	if got := DisableComments(added); len(got) != 3 {
		t.Errorf("%q", got)
	}
}

const goTestJSON = `{"Action":"run","Package":"shop/users/api","Test":"TestA"}
{"Action":"pass","Package":"shop/users/api","Test":"TestA"}
{"Action":"run","Package":"shop/users/api","Test":"TestB"}
{"Action":"fail","Package":"shop/users/api","Test":"TestB"}
{"Action":"skip","Package":"shop/users/api","Test":"TestC"}
{"Action":"pass","Package":"shop/users/api","Test":"TestD/sub"}
{"Action":"fail","Package":"shop/users/api"}
{"Action":"output","Package":"shop/users/bad","Output":"build failed"}
{"Action":"fail","Package":"shop/users/bad"}
not json
`

func TestParseGoTest(t *testing.T) {
	r := ParseGoTest(goTestJSON)
	if r.Tests["shop/users/api TestA"] != Pass || r.Tests["shop/users/api TestB"] != Fail || r.Tests["shop/users/api TestC"] != Skip || r.Tests["shop/users/api TestD/sub"] != Pass {
		t.Errorf("%+v", r.Tests)
	}
	if strings.Join(r.BrokenPackages, ",") != "shop/users/bad" {
		t.Errorf("a package that fails with no failing test is broken: %v", r.BrokenPackages)
	}
}

func TestCompareTests(t *testing.T) {
	base := TestRun{Tests: map[string]string{"p TestOld": Fail, "p TestKeep": Pass, "p TestGone": Pass, "p TestSkipped": Pass, "p TestFlip": Pass}}
	now := TestRun{Tests: map[string]string{"p TestOld": Fail, "p TestKeep": Pass, "p TestSkipped": Skip, "p TestFlip": Fail, "p TestNew": Fail}, BrokenPackages: []string{"q"}}
	v := CompareTests(base, now)
	if strings.Join(v.Regressed, ",") != "p TestFlip" || strings.Join(v.NewFailing, ",") != "p TestNew" ||
		strings.Join(v.Deleted, ",") != "p TestGone" || strings.Join(v.Skipped, ",") != "p TestSkipped" ||
		strings.Join(v.BaseFailing, ",") != "p TestOld" || strings.Join(v.Broken, ",") != "q" {
		t.Errorf("%+v", v)
	}
	if v.Green() {
		t.Error("not green")
	}
	if !CompareTests(base, TestRun{Tests: map[string]string{"p TestOld": Fail, "p TestKeep": Pass, "p TestGone": Pass, "p TestSkipped": Pass, "p TestFlip": Pass}}).Green() {
		t.Error("only base failures: green")
	}
}

func TestSplitTestID(t *testing.T) {
	pkg, name := SplitTestID("shop/users/api TestD/sub")
	if pkg != "shop/users/api" || name != "TestD/sub" {
		t.Errorf("%q %q", pkg, name)
	}
	if RunPattern("TestD/sub") != "^TestD$/^sub$" || RunPattern("TestA") != "^TestA$" {
		t.Errorf("%q %q", RunPattern("TestD/sub"), RunPattern("TestA"))
	}
}
