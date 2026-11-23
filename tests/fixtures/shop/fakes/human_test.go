package fakes

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// bareWithBranch makes a bare repo whose main has a.txt and whose branch head adds file.
func bareWithBranch(t *testing.T, head, file string) string {
	t.Helper()
	work, bare := t.TempDir(), filepath.Join(t.TempDir(), "r.git")
	gitT(t, work, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "-A")
	gitT(t, work, "commit", "-q", "-m", "base")
	gitT(t, work, "switch", "-q", "-c", head)
	if err := os.WriteFile(filepath.Join(work, file), []byte("change\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "-A")
	gitT(t, work, "commit", "-q", "-m", "change")
	gitT(t, work, "clone", "-q", "--bare", work, bare)
	return bare
}

func TestHumanReviewsChecksAndMerges(t *testing.T) {
	s, _ := storeShim(t)
	bare := bareWithBranch(t, "athel/CHG-001-x", "b.txt")
	if _, err := runGH(t, s, "pr create --repo acme/users --head athel/CHG-001-x --base main --title feat:_x --body b --draft"); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge("acme/users", 901, bare, "2026-11-24T10:00:00Z"); err == nil || !strings.Contains(err.Error(), "only an open, ready PR merges") {
		t.Errorf("a draft never merges: %v", err)
	}
	if _, err := runGH(t, s, "pr ready 901 --repo acme/users"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetReview("acme/users", 901, "APPROVED"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetChecks("acme/users", 901, "SUCCESS"); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge("acme/users", 901, bare, "2026-11-24T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	out, err := runGH(t, s, "pr view 901 --repo acme/users")
	for _, want := range []string{`"reviewDecision": "APPROVED"`, `"conclusion": "SUCCESS"`, `"state": "merged"`, `"mergedAt": "2026-11-24T10:00:00Z"`} {
		if err != nil || !strings.Contains(out, want) {
			t.Errorf("pr view lacks %s: %s %v", want, out, err)
		}
	}
	if files := gitT(t, bare, "ls-tree", "--name-only", "main"); files != "a.txt\nb.txt" {
		t.Errorf("main after the merge holds %q", files)
	}
	if subject := gitT(t, bare, "log", "-1", "--format=%s", "main"); subject != "Merge pull request #901 from athel/CHG-001-x" {
		t.Errorf("merge commit %q", subject)
	}
	if err := s.Merge("acme/users", 901, bare, "2026-11-24T11:00:00Z"); err == nil {
		t.Error("a merged PR does not merge twice")
	}
	if err := s.SetReview("acme/users", 999, "APPROVED"); err == nil {
		t.Error("an unknown PR is an error")
	}
}

func TestMergeFailureLeavesThePROpen(t *testing.T) {
	s, _ := storeShim(t)
	bare := bareWithBranch(t, "athel/CHG-001-x", "b.txt")
	if _, err := runGH(t, s, "pr create --repo acme/users --head athel/CHG-001-gone --base main --title feat:_x --body b"); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge("acme/users", 901, bare, "2026-11-24T10:00:00Z"); err == nil || !strings.Contains(err.Error(), "origin/athel/CHG-001-gone") {
		t.Errorf("a missing head fails the merge: %v", err)
	}
	if out, _ := runGH(t, s, "pr view 901 --repo acme/users"); !strings.Contains(out, `"state": "open"`) {
		t.Errorf("a failed merge leaves the PR open: %s", out)
	}
}

func TestLabels(t *testing.T) {
	s, _ := storeShim(t)
	steps := []struct {
		line, want, errText string
	}{
		{"pr create --repo acme/users --head h --base main --title t --body b --label athel:CHG-001", "", "could not add label: 'athel:CHG-001' not found"},
		{"label list --repo acme/users --json name", "[]", ""},
		{"label create athel:CHG-001 --repo acme/users --color 5319e7 --description Athel_change", "", ""},
		{"label create athel:CHG-001 --repo acme/users", "", "already exists"},
		{"label create athel:CHG-001 --repo acme/users --force", "", ""},
		{"label list --repo acme/users --json name", `"name": "athel:CHG-001"`, ""},
		{"label list --repo acme/orders --json name", "[]", ""},
		{"pr create --repo acme/users --head h --base main --title t --body b --label athel:CHG-001", "/pull/901", ""},
		{"pr view 901 --repo acme/users", `"name": "athel:CHG-001"`, ""},
		{"pr create --repo acme/users --head h2 --base main --title t --body b", "/pull/902", ""},
		{"pr edit 902 --repo acme/users --add-label athel:CHG-001", "", ""},
		{"pr edit 902 --repo acme/users --add-label athel:CHG-001", "", ""},
		{"pr view 902 --repo acme/users", `"name": "athel:CHG-001"`, ""},
		{"pr edit 902 --repo acme/users --add-label nope", "", "'nope' not found"},
	}
	for _, st := range steps {
		out, err := runGH(t, s, st.line)
		if st.errText != "" {
			if err == nil || !strings.Contains(err.Error(), st.errText) {
				t.Fatalf("gh %s: err %v, want %q", st.line, err, st.errText)
			}
			continue
		}
		if err != nil || !strings.Contains(out, st.want) {
			t.Fatalf("gh %s = %q, %v; want %q", st.line, out, err, st.want)
		}
	}
	if out, _ := runGH(t, s, "pr view 902 --repo acme/users"); strings.Count(out, "athel:CHG-001") != 1 {
		t.Errorf("adding a label twice keeps one: %s", out)
	}
}

func TestLabelStoreErrors(t *testing.T) {
	s, _, _ := shim(t)
	if _, err := runGH(t, s, "label create x --repo acme/users"); err == nil || !strings.Contains(err.Error(), "no label store") {
		t.Errorf("creating needs a state folder: %v", err)
	}
	s.State = t.TempDir()
	if err := os.WriteFile(filepath.Join(s.State, "labels.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"label list --repo acme/users", "label create x --repo acme/users", "pr create --repo acme/users --head h --base main --title t --body b --label x"} {
		if _, err := runGH(t, s, line); err == nil || !strings.Contains(err.Error(), "labels.json") {
			t.Errorf("gh %s with a broken store: %v", line, err)
		}
	}
}
