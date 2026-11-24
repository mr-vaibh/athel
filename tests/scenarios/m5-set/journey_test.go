// Package m5set is M5's scripted journey (M5-plan task 14): a 3-service change taken through the
// real CLI with no model, on the shop fixture with its runnable services. The script plays every
// role the way the agents and the developer would, through the same commands: the architect's
// documents, the developer's commits, the checkers' reports, the developer's prompts, and at the
// end the human who merges. It proves, for free, what the M5 evals then measure with real
// sessions: the change set, linked PRs, the merge-order rehearsal, status and the follow-up.
package m5set

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mr-vaibh/athel/tests/fixtures/shop"
	"github.com/mr-vaibh/athel/tests/internal/evalkit"
)

const (
	change  = "CHG-001"
	session = "sess-journey"
)

type journey struct {
	t  *testing.T
	sb *evalkit.Sandbox
}

// athel runs the CLI in the session and returns its data, failing the test on an error.
func (j *journey) athel(stdin string, args ...string) map[string]any {
	j.t.Helper()
	d, err := j.try(stdin, args...)
	if err != nil {
		j.t.Fatal(err)
	}
	return d
}

func (j *journey) try(stdin string, args ...string) (map[string]any, error) {
	out, err := j.sb.Athel(stdin, append([]string{"--json", "--session", session}, args...)...)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	return env.Data, json.Unmarshal(out, &env)
}

// prompt is the developer typing a slash command: the prompt hook records the grant.
func (j *journey) prompt(text string) {
	j.t.Helper()
	in, _ := json.Marshal(map[string]any{"session_id": session, "prompt_id": "p-" + text, "prompt": text, "cwd": j.sb.WS}) // strings always encode
	if _, err := j.sb.Athel(string(in), "hook", "user-prompt-submit"); err != nil {
		j.t.Fatal(err)
	}
}

func (j *journey) to(state string) {
	j.t.Helper()
	by := "athel"
	if state == "plan_approved" {
		by = "human"
	}
	j.athel("", "state", "transition", change, "--to", state, "--by", by, "--reason", "journey")
}

// commit writes files in Athel's worktree of repo and commits them with the task's trailers;
// Athel's git hooks lint the commit as they would the developer agent's.
func (j *journey) commit(repo, task, msg string, files map[string]string) {
	j.t.Helper()
	tree := filepath.Join(j.sb.WS, "repos", repo+"@"+change)
	for name, body := range files {
		p := filepath.Join(tree, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			j.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			j.t.Fatal(err)
		}
	}
	full := fmt.Sprintf("%s\n\nAthel-Change: %s\nAthel-Task: %s", msg, change, task)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", full}} {
		cmd := exec.Command("git", append([]string{"-C", tree}, args...)...)
		cmd.Env = []string{"PATH=" + j.sb.SessionPath(), "HOME=" + j.sb.Home, "ATHEL_HOME=" + j.sb.AthelHome, "GOTOOLCHAIN=local"}
		if out, err := cmd.CombinedOutput(); err != nil {
			j.t.Fatalf("git %v in %s: %v\n%s", args, repo, err, out)
		}
	}
}

// task builds one task as the build loop does: the developer's commit, QA and security reports,
// an approved review round, a green full gate run, done.
func (j *journey) task(id, repo, msg, test string, files map[string]string) {
	j.t.Helper()
	j.athel("", "state", "enter", change, "build", "--task", id)
	j.athel("", "gate", "baseline", change, id)
	j.commit(repo, id, msg, files)
	j.athel("cases: [{category: boundary, file: "+test+", name: TestEdge, result: pass}]\nsuite: {command: go test ./..., result: pass}\ngaps: []\nfindings: []\n",
		"qa", "save", change, id, "-")
	j.athel("checked: [input validation]\nfindings: []\n", "security", "save", change, id, "-")
	j.athel("verdict: approved\n", "review", "save", change, id, "-")
	run := j.athel("", "gate", "run", "--stage", "full", change, id)
	if run["green"] != true {
		b, _ := json.MarshalIndent(run, "", " ") // a plain map always encodes
		j.t.Fatalf("%s gates not green:\n%s", id, b)
	}
	j.athel("", "build", "done", change, id)
	j.athel("", "state", "exit", change, "build", "--result", "ok")
}

func TestThreeRepoJourney(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the fixture and runs every gate in three repos")
	}
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		t.Fatal("the journey runs every gate, lint included: golangci-lint must be on PATH (docs/context/environment.md)")
	}
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := evalkit.BuildTools(repo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := evalkit.NewSandbox(t.TempDir(), tl)
	if err != nil {
		t.Fatal(err)
	}
	if err := shop.SeedSystem(sb.Layout); err != nil {
		t.Fatal(err)
	}
	j := &journey{t: t, sb: sb}
	j.specify()
	j.build()
	j.ship()
	j.merge()
}
