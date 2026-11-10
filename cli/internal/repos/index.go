package repos

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// SchemaID is the schema of .athel/repos/index.yaml.
const SchemaID = schema.RepoIndex

// StaleAfter is when SessionStart rebuilds the index (doc 03 step 5).
const StaleAfter = 7 * 24 * time.Hour

// Index is the repo-index@1 document.
type Index struct {
	Schema    string   `yaml:"schema" json:"schema"`
	IndexedAt string   `yaml:"indexed_at" json:"indexed_at"`
	Roots     []string `yaml:"roots" json:"roots"`
	Repos     []Entry  `yaml:"repos" json:"repos"`
}

// Entry is one local clone.
type Entry struct {
	Remote        *string `yaml:"remote" json:"remote"`
	Path          string  `yaml:"path" json:"path"`
	DefaultBranch *string `yaml:"default_branch" json:"default_branch"`
	LastCommitAt  *string `yaml:"last_commit_at" json:"last_commit_at"`
	Fact          *string `yaml:"fact" json:"fact"`
}

// Read describes one clone from its .git folder and a read-only git log.
func Read(dir string) Entry {
	e := Entry{Path: dir}
	gitDir := filepath.Join(dir, ".git")
	if remote := gitro.NormaliseRemote(originURL(gitDir)); remote != "" {
		e.Remote = &remote
	}
	if b, err := os.ReadFile(filepath.Join(gitDir, "refs", "remotes", "origin", "HEAD")); err == nil { //nolint:gosec // a fixed file inside a clone the developer confirmed
		if branch, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/remotes/origin/"); ok && branch != "" {
			e.DefaultBranch = &branch
		}
	}
	if code, out, err := gitro.Run(dir, "log", "-1", "--format=%cI"); err == nil && code == 0 {
		if t, err := clock.Parse(strings.TrimSpace(out)); err == nil {
			at := clock.Format(t)
			e.LastCommitAt = &at
		}
	}
	return e
}

var (
	sectionRe = regexp.MustCompile(`^\s*\[\s*remote\s+"origin"\s*\]\s*$`)
	urlRe     = regexp.MustCompile(`^\s*url\s*=\s*(.+?)\s*$`)
)

// originURL reads [remote "origin"] url from .git/config without running git.
func originURL(gitDir string) string {
	b, err := os.ReadFile(filepath.Join(gitDir, "config")) //nolint:gosec // a fixed file inside a clone the developer confirmed
	if err != nil {
		return ""
	}
	in := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			in = sectionRe.MatchString(line)
			continue
		}
		if m := urlRe.FindStringSubmatch(line); in && m != nil {
			return strings.Trim(m[1], `"`)
		}
	}
	return ""
}

// Build indexes the clones under the expanded roots. It reports whether the walk finished.
func Build(roots []string, now time.Time, deadline time.Time, clk func() time.Time) (Index, bool) {
	dirs, complete := Walk(roots, deadline, clk)
	idx := Index{Schema: SchemaID, IndexedAt: clock.Format(now), Roots: roots, Repos: make([]Entry, 0, len(dirs))}
	for _, d := range dirs {
		idx.Repos = append(idx.Repos, Read(d))
	}
	return idx, complete
}

// Save validates and atomically writes the index.
func Save(reg *schema.Registry, path string, idx Index) error {
	b, _ := yaml.Marshal(idx) // strings and lists of them: always encodes
	if err := reg.ValidateYAML(SchemaID, b); err != nil {
		return err
	}
	if err := fsutil.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, b)
}

// Load reads the index; ok is false when there is none yet.
func Load(reg *schema.Registry, path string) (Index, bool, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the workspace's own index file
	if errors.Is(err, fs.ErrNotExist) {
		return Index{}, false, nil
	}
	if err != nil {
		return Index{}, false, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	if err := reg.ValidateYAML(SchemaID, b); err != nil {
		return Index{}, false, err
	}
	var idx Index
	_ = yaml.Unmarshal(b, &idx) // the schema has just accepted every field and type
	return idx, true, nil
}

// Resolve finds the clone a recipe means by repo: an absolute path to a checkout, a normalised
// remote, or a remote's last part when exactly one indexed remote ends with it. With several
// clones of one remote, the one with the newest commit is read (doc 03 rules).
func (idx Index) Resolve(repo string) (string, error) {
	if filepath.IsAbs(repo) {
		if isClone(repo) {
			return repo, nil
		}
		return "", aerr.New(aerr.StateNotFound, repo+" is not a git checkout")
	}
	_, local, err := idx.Lookup(repo)
	if err != nil {
		return "", err
	}
	if local == "" {
		return "", aerr.New(aerr.StateNotFound, "repo "+repo+" is not in the repo index").
			WithNextAction("run athel repos index, or clone it into Athel's cache")
	}
	return local, nil
}

// Lookup finds the remote a name means (a host/org/repo remote, or the last part of exactly one
// indexed remote) and its newest local clone, "" when there is none.
func (idx Index) Lookup(name string) (remote, local string, err error) {
	var match *Entry
	remotes := map[string]bool{}
	for i := range idx.Repos {
		e := &idx.Repos[i]
		if e.Remote == nil || (*e.Remote != name && !strings.HasSuffix(*e.Remote, "/"+name)) {
			continue
		}
		remotes[*e.Remote] = true
		if match == nil || newer(e, match) {
			match = e
		}
	}
	switch {
	case len(remotes) > 1:
		return "", "", aerr.New(aerr.StateNotFound, "repo "+name+" matches several remotes; name it as host/org/repo")
	case match != nil:
		return *match.Remote, match.Path, nil
	case gitro.NormaliseRemote("https://"+name) == name:
		return name, "", nil
	}
	return "", "", nil
}

func newer(a, b *Entry) bool {
	return a.LastCommitAt != nil && (b.LastCommitAt == nil || *a.LastCommitAt > *b.LastCommitAt)
}

// Stale reports whether the index should be rebuilt.
func (idx Index) Stale(now time.Time) bool {
	at, err := clock.Parse(idx.IndexedAt)
	return err != nil || now.Sub(at) > StaleAfter
}
