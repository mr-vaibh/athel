package gates

import (
	"strings"
	"testing"
)

func TestIsTestFile(t *testing.T) {
	for path, want := range map[string]bool{
		"src/a_test.go": true, "src/a.go": false, "web/x.test.ts": true, "web/x.spec.tsx": true,
		"tests/test_api.py": true, "pkg/api_test.py": true, "test/helpers.js": true, "src/latest.go": false, "README.md": false,
		// JVM, .NET and Ruby services (M5-plan G15): their own naming and folders.
		"src/test/java/com/acme/UserServiceTest.java": true, "src/main/java/com/acme/UserService.java": false,
		"src/test/kotlin/OrdersSpec.kt": true, "app/OrdersIT.java": true, "Users.Tests/UserTests.cs": true,
		"spec/models/user_spec.rb": true, "app/models/user.rb": false, "src/main/java/Contest.java": false,
	} {
		if IsTestFile(path) != want {
			t.Errorf("%s: %v", path, !want)
		}
	}
}

func TestMissingACs(t *testing.T) {
	tests := map[string]string{
		"src/a_test.go": "func TestPhoneValid(t *testing.T) { // AC-1\n",
		"src/b_test.go": "t.Run(\"AC-10 rejects letters\", ...)",
	}
	if got := MissingACs([]string{"AC-1", "AC-2", "AC-10"}, tests); strings.Join(got, ",") != "AC-2" {
		t.Errorf("AC-1 and AC-10 are tested, AC-2 is not, and AC-1 never matches inside AC-10: %v", got)
	}
	if got := MissingACs([]string{"AC-1"}, map[string]string{"x_test.go": "AC-10"}); strings.Join(got, ",") != "AC-1" {
		t.Errorf("%v", got)
	}
	if got := MissingACs(nil, nil); len(got) != 0 {
		t.Errorf("a task that covers no AC: %v", got)
	}
}

func TestOverride(t *testing.T) {
	base := Defaults(Go, nil)
	got, err := Override(base, []Gate{
		{ID: "lint", Command: []string{"make", "lint"}},
		{ID: "integration", Stage: Full, Pass: ExitZero, Waivable: Human, Command: []string{"make", "integration"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Gate{}
	for _, g := range got {
		byID[g.ID] = g
	}
	if strings.Join(byID["lint"].Command, " ") != "make lint" || byID["lint"].Pass != NewFindings || byID["lint"].Waivable != Human {
		t.Errorf("an override replaces only the command: %+v", byID["lint"])
	}
	if g := got[len(got)-1]; g.ID != "integration" || g.Stage != Full {
		t.Errorf("a new gate is added at the end: %+v", g)
	}
	bad := map[string]Gate{
		"loosen":       {ID: "build", Waivable: Human, Command: []string{"x"}},
		"empty":        {ID: "lint"},
		"repass":       {ID: "unit", Pass: ExitZero, Command: []string{"x"}},
		"builtin":      {ID: "contract", Command: []string{"x"}},
		"no command":   {ID: "e2e", Stage: Full, Pass: ExitZero, Waivable: Human},
		"bad pass":     {ID: "e2e", Stage: Full, Pass: Tests, Waivable: Human, Command: []string{"x"}},
		"bad stage":    {ID: "e2e", Stage: "nightly", Pass: ExitZero, Waivable: Human, Command: []string{"x"}},
		"bad waivable": {ID: "e2e", Stage: Full, Pass: ExitZero, Waivable: "anyone", Command: []string{"x"}},
	}
	for name, g := range bad {
		if _, err := Override(base, []Gate{g}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// TestMatcher: a repo's own test patterns add to the defaults, never replace them.
func TestTestMatcher(t *testing.T) {
	m := TestMatcher([]string{"qa/**/*.check.ts"})
	for path, want := range map[string]bool{"qa/orders/flow.check.ts": true, "src/a_test.go": true, "src/flow.check.ts": false, "src/a.go": false} {
		if m(path) != want {
			t.Errorf("%s: %v", path, !want)
		}
	}
	if !TestMatcher(nil)("src/a_test.go") {
		t.Error("no patterns: the defaults")
	}
}
