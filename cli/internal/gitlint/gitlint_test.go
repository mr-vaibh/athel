package gitlint

import (
	"strings"
	"testing"
)

var rules = Rules{Format: "conventional", Trailers: []string{"Athel-Change: {change_id}", "Athel-Task: {task_id}"}, MaxFiles: 15}

func TestMessage(t *testing.T) {
	tasks := []string{"T-01", "T-02"}
	tests := []struct {
		name, msg string
		want      []string
	}{
		{"valid", "feat(users): validate phone numbers\n\nWhy it matters.\n\nAthel-Change: CHG-001\nAthel-Task: T-01\n", nil},
		{"valid without scope or body", "fix: handle empty input\n\nAthel-Change: CHG-001\nAthel-Task: T-02", nil},
		{"breaking marker", "feat(api)!: drop v1\n\nAthel-Change: CHG-001\nAthel-Task: T-01", nil},
		{"not conventional", "Validate phone numbers\n\nAthel-Change: CHG-001\nAthel-Task: T-01", []string{"subject"}},
		{"unknown type", "feature: x\n\nAthel-Change: CHG-001\nAthel-Task: T-01", []string{"subject"}},
		{"empty summary", "feat(users): \n\nAthel-Change: CHG-001\nAthel-Task: T-01", []string{"subject"}},
		{"no trailers", "feat: x\n", []string{"Athel-Change", "Athel-Task"}},
		{"trailers not in the last paragraph", "feat: x\n\nAthel-Change: CHG-001\nAthel-Task: T-01\n\nmore text", []string{"Athel-Change", "Athel-Task"}},
		{"other change", "feat: x\n\nAthel-Change: CHG-002\nAthel-Task: T-01", []string{"Athel-Change: CHG-002"}},
		{"task not in this repo's plan", "feat: x\n\nAthel-Change: CHG-001\nAthel-Task: T-09", []string{"T-09"}},
		{"empty message", "", []string{"empty"}},
		{"comment lines are ignored", "# Please enter the commit message\nfeat: x\n\nAthel-Change: CHG-001\nAthel-Task: T-01\n# On branch athel/x", nil},
	}
	for _, tt := range tests {
		got := Message(tt.msg, rules, "CHG-001", tasks)
		if len(got) != len(tt.want) {
			t.Errorf("%s: problems %q, want %d about %q", tt.name, got, len(tt.want), tt.want)
			continue
		}
		for i, w := range tt.want {
			if !strings.Contains(got[i], w) {
				t.Errorf("%s: problem %q does not mention %q", tt.name, got[i], w)
			}
		}
	}
	free := Rules{Format: "free", Trailers: []string{"Athel-Change: {change_id}"}}
	if got := Message("whatever I like\n\nAthel-Change: CHG-001", free, "CHG-001", nil); len(got) != 0 {
		t.Errorf("free format checks only the trailers: %q", got)
	}
	if got := Message("x\n\nAthel-Change: CHG-001\nCustom: {nothing}", Rules{Trailers: []string{"Custom: {nothing}"}}, "CHG-001", nil); len(got) != 0 {
		t.Errorf("a trailer template without a known placeholder must match literally: %q", got)
	}
	if got := Message("x\n\nCustom: other", Rules{Trailers: []string{"Custom: fixed"}}, "CHG-001", nil); len(got) != 1 || !strings.Contains(got[0], "should be fixed") {
		t.Errorf("a literal trailer with another value: %q", got)
	}
}

func TestTitle(t *testing.T) {
	if p := Title("feat(users): validate phone numbers", "conventional"); len(p) != 0 {
		t.Errorf("%q", p)
	}
	if p := Title("Validate phone numbers", "conventional"); len(p) != 1 {
		t.Errorf("%q", p)
	}
	if p := Title("Anything", "free"); len(p) != 0 {
		t.Errorf("%q", p)
	}
	if p := Title(" ", "free"); len(p) != 1 {
		t.Errorf("an empty title is refused: %q", p)
	}
}

func TestBranch(t *testing.T) {
	tmpl := "athel/{change_id}-{slug}"
	for name, ok := range map[string]bool{
		"athel/CHG-001-phone-update":   true,
		"athel/CHG-001-phone-update-2": true,
		"athel/CHG-002-phone-update":   false,
		"feature/CHG-001-x":            false,
		"athel/CHG-001-":               false,
		"athel/CHG-001-Phone":          false,
	} {
		if got := len(Branch(name, tmpl, "CHG-001")) == 0; got != ok {
			t.Errorf("Branch(%q) ok = %v, want %v", name, got, ok)
		}
	}
	if p := Branch("athel/CHG-001-fix-SF-3", "athel/{change_id}-fix-{finding_id}", "CHG-001"); len(p) != 0 {
		t.Errorf("side-finding branch: %q", p)
	}
}

func TestStaged(t *testing.T) {
	small := Staged{Path: "a.go", Size: 10, Added: "x := 1\n"}
	var many []Staged
	for range 16 {
		many = append(many, small)
	}
	if p := StagedFiles(many, rules); len(p) != 1 || !strings.Contains(p[0], "16 files") {
		t.Errorf("too many files: %q", p)
	}
	big := Staged{Path: "data.bin", Size: MaxFileBytes + 1}
	if p := StagedFiles([]Staged{small, big}, rules); len(p) != 1 || !strings.Contains(p[0], "data.bin") || !strings.Contains(p[0], "1 MB") {
		t.Errorf("big file: %q", p)
	}
	secret := Staged{Path: "config.go", Size: 50, Added: `token := "ghp_0123456789abcdefghijABCDEFGHIJ012345"` + "\n"}
	p := StagedFiles([]Staged{secret}, rules)
	if len(p) != 1 || !strings.Contains(p[0], "config.go") || !strings.Contains(p[0], "github-token") || strings.Contains(p[0], "ghp_") {
		t.Errorf("secret: %q", p)
	}
	if p := StagedFiles([]Staged{small}, Rules{}); len(p) != 0 {
		t.Errorf("no file limit configured: %q", p)
	}
}
