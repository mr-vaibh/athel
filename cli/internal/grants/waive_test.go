package grants

import (
	"strings"
	"testing"
)

func TestParseWaivePrompt(t *testing.T) {
	w, ok := ParseWaivePrompt("/athel:waive CHG-001 T-01 lint until=2026-12-01 'legacy code, fixed in SHOP-77'")
	if !ok || w.Change != "CHG-001" || w.Task != "T-01" || w.Subject != "lint" || w.Until != "2026-12-01" || w.Reason != "legacy code, fixed in SHOP-77" {
		t.Fatalf("%+v %v", w, ok)
	}
	if f, ok := ParseWaivePrompt("  /athel:waive CHG-010 T-12 R2-04 until=2026-11-30 'accepted risk'  "); !ok || f.Subject != "R2-04" {
		t.Errorf("a finding: %+v %v", f, ok)
	}
	for _, bad := range []string{
		"/athel:waive CHG-001 T-01 lint until=2026-12-01",
		"/athel:waive CHG-001 T-01 lint 'no expiry'",
		"/athel:waive CHG-001 lint until=2026-12-01 'no task'",
		"/athel:waive CHG-001 T-01 lint until=tomorrow 'x'",
		"/athel:waive CHG-001 T-01 lint until=2026-12-01 'it''s quoted'",
		"/athel:waive CHG-001 T-01 lint until=2026-12-01 '$(rm -rf /)'",
		"/athel:waive CHG-001 T-01 lint until=2026-12-01 ''",
		"/athel:waive CHG-001 T-01 Lint! until=2026-12-01 'x'",
		"please /athel:waive CHG-001 T-01 lint until=2026-12-01 'x'",
	} {
		if _, ok := ParseWaivePrompt(bad); ok {
			t.Errorf("%q must not parse", bad)
		}
	}
}

func TestWaiveKeyAndCommand(t *testing.T) {
	w := Waiver{Change: "CHG-001", Task: "T-01", Subject: "lint", Until: "2026-12-01", Reason: "legacy code"}
	other := w
	other.Reason = "another reason"
	if WaiveKey(w) == WaiveKey(other) || !strings.HasPrefix(WaiveKey(w), "waive:") {
		t.Error("the key binds every field")
	}
	if got := WaiveCommand(w); got != "athel gate waive CHG-001 T-01 lint --until 2026-12-01 --reason 'legacy code'" {
		t.Errorf("%q", got)
	}
}
