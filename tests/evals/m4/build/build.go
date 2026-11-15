// Package build is M4's build eval (EC-1, M4-plan §4.10): one-repo PRDs taken from start to a
// PR by real Claude Code sessions, as a developer would. A run is valid only if every check
// passes, all of them deterministic, and the grader re-runs the gates itself rather than trust
// a self-reported green.
package build

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Change is the first change of a fresh workspace.
const Change = "CHG-001"

// MaxLines is git.yaml's PR size limit (FR-GIT-009).
const MaxLines = 400

// Planted is a nearby bug outside the PRD's scope.
type Planted struct {
	File string `yaml:"file"`
	Text string `yaml:"text"`
}

// Case is one PRD.
type Case struct {
	ID      string            `yaml:"id"`
	PRD     string            `yaml:"prd"`
	Repo    string            `yaml:"repo"`
	Kinds   []string          `yaml:"kinds"`
	Seed    map[string]string `yaml:"seed"`
	Planted Planted           `yaml:"planted"`
}

func here() string {
	_, self, _, _ := runtime.Caller(0) // this file's own path is always known
	return filepath.Dir(self)
}

// Load reads cases.yaml; PRD paths become absolute.
func Load() ([]Case, error) {
	b, err := os.ReadFile(filepath.Join(here(), "cases.yaml"))
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var doc struct {
		Cases []Case `yaml:"cases"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("cases.yaml: %w", err)
	}
	for i := range doc.Cases {
		doc.Cases[i].PRD = filepath.Join(here(), doc.Cases[i].PRD)
	}
	return doc.Cases, nil
}

// git runs git in dir with a fixed identity and no global config surprises.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=Mate", "-c", "user.email=mate@example.invalid", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // git with the grader's own arguments
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %v in %s: %w: %s", args, dir, err, out)
	}
	return string(out), nil
}

// Seed commits the case's seed files in work, a clone of the repo, and pushes them to its main
// as a teammate's earlier work, before the run starts. run is git in a folder.
func Seed(work string, c Case, run func(dir string, args ...string) (string, error)) error {
	names := make([]string, 0, len(c.Seed))
	for name, body := range c.Seed {
		p := filepath.Join(work, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			return err
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, args := range [][]string{append([]string{"add", "--"}, names...), {"commit", "-q", "-m", "Add helpers"}, {"push", "-q", "origin", "HEAD"}} {
		if _, err := run(work, args...); err != nil {
			return err
		}
	}
	return nil
}

// TreeHash is a digest of every file under dir, with its path and mode, for the check that the
// developer's clones are never changed (FR-GIT-001).
func TreeHash(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)                                     // p is under dir
		_, _ = fmt.Fprintf(h, "%s %v %d\n", rel, info.Mode(), info.Size()) // a hash never fails to write
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(p) //nolint:gosec // a file of the run's own sandbox
			if err != nil {
				return err
			}
			h.Write(b)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// Gates are what the grader re-runs on a fresh clone of the pushed branch: doc 06's format,
// lint (zero new errors versus base), build, types and unit gates for the Go repos.
func Gates(base string) [][]string {
	return [][]string{
		{"gofmt", "-l", "."},
		{"go", "vet", "./..."},
		{"go", "build", "./..."},
		{"go", "test", "-count=1", "./..."},
		{"golangci-lint", "run", "--new-from-rev", base, "./..."},
	}
}

// RerunGates runs Gates in dir, a git checkout whose history holds base, and lists every one
// that fails; a tool that is not installed is a problem too, so a run is never valid on fewer
// gates than it claims.
func RerunGates(dir, base string, env []string) []string {
	var problems []string
	for _, g := range Gates(base) {
		if _, err := exec.LookPath(g[0]); err != nil {
			problems = append(problems, g[0]+" is not installed, so its gate cannot be re-run")
			continue
		}
		cmd := exec.Command(g[0], g[1:]...) //nolint:gosec // the fixed gate list
		cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil || (g[0] == "gofmt" && strings.TrimSpace(string(out)) != "") {
			problems = append(problems, fmt.Sprintf("gate %s fails on the pushed head: %v %s", strings.Join(g, " "), err, firstLines(string(out), 5)))
		}
	}
	return problems
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(strings.TrimSpace(s), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, " | ")
}

// ChangedLines counts the lines a branch changes against base, without lockfiles.
func ChangedLines(dir, base, head string) (int, error) {
	out, err := git(dir, "diff", "--numstat", base+"..."+head)
	if err != nil {
		return 0, err
	}
	total := 0
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || strings.HasSuffix(f[2], ".lock") || strings.HasSuffix(f[2], "go.sum") {
			continue
		}
		for _, n := range f[:2] {
			v, _ := strconv.Atoi(n) // binary files show "-"
			total += v
		}
	}
	return total, nil
}

// PRCall is one line of the fake GitHub's call log.
type PRCall struct {
	Args []string `json:"args"`
}

// ReadCalls reads gh-calls.jsonl.
func ReadCalls(state string) ([]PRCall, error) {
	b, err := os.ReadFile(filepath.Join(state, "gh-calls.jsonl")) //nolint:gosec // the run's own fake GitHub state
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []PRCall
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var c PRCall
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
