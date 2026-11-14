package fakes

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// storeShim is a shim whose gh keeps pull requests in a fresh state folder.
func storeShim(t *testing.T) (Shim, string) {
	t.Helper()
	s, _, _ := shim(t)
	s.State = t.TempDir()
	return s, s.State
}

func runGH(t *testing.T, s Shim, line string) (string, error) {
	t.Helper()
	var out, errOut strings.Builder
	s.Out, s.Err = &out, &errOut
	if code := s.Run(append([]string{"gh"}, strings.Fields(line)...)); code != 0 {
		return out.String(), &ghError{errOut.String()}
	}
	return out.String(), nil
}

type ghError struct{ msg string }

func (e *ghError) Error() string { return e.msg }

func TestFakePRStore(t *testing.T) {
	s, state := storeShim(t)
	body := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(body, []byte("## Summary\nPhone format."), 0o600); err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		line string
		want string
	}{
		{"pr create --repo acme/orders --head athel/CHG-001-x --base main --title feat(orders):_x --body-file " + body + " --draft", "https://github.com/acme/orders/pull/901"},
		{"pr view 901 --repo acme/orders --json isDraft", `"isDraft": true`},
		{"pr list --repo acme/orders", `"number": 901`},
		{"pr edit 901 --repo acme/orders --title feat(orders):_y", ""},
		{"pr view 901 --repo acme/orders", `"title": "feat(orders):_y"`},
		{"pr ready 901 --repo acme/orders", ""},
		{"pr view 901 --repo acme/orders", `"isDraft": false`},
		{"pr view 901 --repo acme/orders", `"body": "## Summary\nPhone format."`},
		{"pr create --repo acme/users --head athel/CHG-002-y --base main --title fix:_z --body b", "https://github.com/acme/users/pull/902"},
		{"pr view 812 --repo acme/users", `"number": 812`},
	}
	for _, st := range steps {
		out, err := runGH(t, s, st.line)
		if err != nil || !strings.Contains(out, st.want) {
			t.Fatalf("gh %s = %q, %v; want %q", st.line, out, err, st.want)
		}
	}
	if _, err := runGH(t, s, "pr merge 901 --repo acme/orders --merge"); err == nil || !strings.Contains(err.Error(), "never merges") {
		t.Errorf("merge must fail: %v", err)
	}
	calls := readCalls(t, state)
	if len(calls) != len(steps)+1 || !strings.HasPrefix(calls[len(calls)-1], "pr merge 901") {
		t.Errorf("every gh call is logged, the merge too: %v", calls)
	}
}

func readCalls(t *testing.T, state string) []string {
	t.Helper()
	f, err := os.Open(filepath.Join(state, "gh-calls.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var calls []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c struct {
			Args []string `json:"args"`
		}
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, strings.Join(c.Args, " "))
	}
	return calls
}

func TestFakePRStoreErrors(t *testing.T) {
	s, state := storeShim(t)
	for _, line := range []string{
		"pr create --repo acme/orders --head h --title t --body b",
		"pr create --repo acme/orders --base main --title t --body b",
		"pr create --repo acme/orders --head h --base main --body b",
		"pr create --repo acme/orders --head h --base main --title t --body-file /nonexistent/body.md",
		"pr ready 999 --repo acme/orders",
		"pr edit 999 --repo acme/orders --title t",
		"pr ready --repo acme/orders",
	} {
		if _, err := runGH(t, s, line); err == nil {
			t.Errorf("gh %s: want an error", line)
		}
	}
	noStore, _, _ := shim(t)
	if _, err := runGH(t, noStore, "pr create --repo acme/orders --head h --base main --title t --body b"); err == nil || !strings.Contains(err.Error(), "SHOP_GH_STATE") {
		t.Errorf("without a state folder writes fail: %v", err)
	}
	if err := os.WriteFile(filepath.Join(state, "prs.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runGH(t, s, "pr list --repo acme/orders"); err == nil {
		t.Error("a broken store must fail")
	}
	s.State = filepath.Join(state, "missing", "deeper")
	if err := os.WriteFile(filepath.Join(state, "missing"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runGH(t, s, "auth status"); err == nil {
		t.Error("an unwritable call log must fail the call")
	}
}

func TestFakePREditBody(t *testing.T) {
	s, _ := storeShim(t)
	body := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(body, []byte("from a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, st := range []struct{ line, want string }{
		{"pr create --repo acme/orders --head h --base main --title t --body first", "pull/901"},
		{"pr edit 901 --repo acme/orders --body second", ""},
		{"pr view 901 --repo acme/orders", `"body": "second"`},
		{"pr edit 901 --repo acme/orders --body-file " + body, ""},
		{"pr view 901 --repo acme/orders", `"body": "from a file"`},
	} {
		if out, err := runGH(t, s, st.line); err != nil || !strings.Contains(out, st.want) {
			t.Fatalf("gh %s = %q, %v; want %q", st.line, out, err, st.want)
		}
	}
	if _, err := runGH(t, s, "pr edit 901 --repo acme/orders --body-file /nonexistent/body.md"); err == nil {
		t.Error("a missing body file must fail the edit")
	}
}

func TestFakePRStoreFolderErrors(t *testing.T) {
	s, state := storeShim(t)
	if err := os.Mkdir(filepath.Join(state, prsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runGH(t, s, "pr view 812 --repo acme/users"); err == nil {
		t.Error("an unreadable store must fail")
	}
	if _, err := runGH(t, s, "pr create --repo acme/orders --head h --base main --title t --body b"); err == nil {
		t.Error("an unreadable store must fail a create")
	}
	gone, _ := storeShim(t)
	gone.State = filepath.Join(t.TempDir(), "gone")
	if err := gone.updatePRs(func(p []storedPR) ([]storedPR, error) { return p, nil }); err == nil {
		t.Error("a missing state folder must fail the save")
	}
	if err := os.RemoveAll(filepath.Join(state, prsFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(state, prsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, prsFile, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.savePRs(nil); err == nil {
		t.Error("renaming over a non-empty folder must fail")
	}
}
