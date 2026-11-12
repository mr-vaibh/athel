package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/clock"
	"github.com/mr-vaibh/athel/cli/internal/conventions"
	"github.com/mr-vaibh/athel/cli/internal/digest"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
	"github.com/mr-vaibh/athel/cli/internal/lock"
	"github.com/mr-vaibh/athel/cli/internal/repos"
	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// resolveRepo names a repo by its remote: an indexed clone's name ("users") through the repo
// index, or a git URL or host/org/repo as given. It never touches the network.
func (r *runCtx) resolveRepo(name string) string {
	if ws, err := r.workspaceDir(); err == nil {
		if idx, _, err := repos.Load(r.registry(), ws.RepoIndexFile()); err == nil {
			if remote, _, err := idx.Lookup(name); err == nil && remote != "" {
				return remote
			}
		}
	}
	return conventions.Remote(name, gitro.NormaliseRemote)
}

// repoDir is .athel/repos/<repo>/ for a normalised remote.
func (r *runCtx) repoDir(remote string) (string, error) {
	ws, err := r.workspaceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(ws.Dir(), "repos", conventions.Dir(remote)), nil
}

// writeRepoFiles writes files into .athel/repos/<repo>/ under the workspace lock.
func (r *runCtx) writeRepoFiles(remote string, files map[string][]byte) (string, error) {
	dir, err := r.repoDir(remote)
	if err != nil {
		return "", err
	}
	ws, err := r.workspaceDir()
	if err != nil {
		return "", err
	}
	l, err := lock.Acquire(ws.LockFile(), state.DefaultLockTimeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = l.Release() }() // the writes' errors are what matter
	if err := fsutil.EnsureDir(dir); err != nil {
		return "", err
	}
	for name, data := range files {
		if err := fsutil.WriteFileAtomic(filepath.Join(dir, name), data); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// cmdReposSurvey collects onboarding inputs from Athel's cache clone, never the developer's
// (FR-SPEC-006), and keeps them for `conventions save`.
func cmdReposSurvey(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("repos survey"), args, 1)
	if err != nil {
		return nil, err
	}
	c, err := r.cacheClone(pos[0])
	if err != nil {
		return nil, err
	}
	s, err := conventions.Take(gitro.Run, c.Remote, c.Path)
	if err != nil {
		return nil, aerr.Wrap(aerr.ToolUnreachable, "survey "+c.Remote, err)
	}
	b, _ := json.Marshal(s) // strings and lists only: always encodes
	if _, err := r.writeRepoFiles(c.Remote, map[string][]byte{"survey.json": b}); err != nil {
		return nil, err
	}
	return s, nil
}

type conventionsDoc struct {
	Conventions string `yaml:"conventions"`
	Map         string `yaml:"map"`
}

type conventionsSaved struct {
	Repo string `json:"repo"`
	Dir  string `json:"dir"`
}

// cmdConventionsSave stores the architect's conventions.md and map.md (stdin YAML with both, so
// it needs no file write) with what they were written from. For an existing repo that is the
// latest survey; --new marks a new project, whose conventions only age.
func cmdConventionsSave(r *runCtx, args []string) (any, error) {
	fl := r.newFlags("conventions save")
	isNew := fl.Bool("new", false, "a new project with no code yet")
	pos, err := parse(fl, args, 2)
	if err != nil {
		return nil, err
	}
	data, err := r.readBlock(pos[1])
	if err != nil {
		return nil, err
	}
	var doc conventionsDoc
	if err := decodeStrict(data, &doc, "conventions"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(doc.Conventions) == "" || strings.TrimSpace(doc.Map) == "" {
		return nil, aerr.New(aerr.StateInvalid, "both conventions and map are required")
	}
	remote := r.resolveRepo(pos[0])
	meta := conventions.Meta{Schema: schema.Conventions, Repo: remote, NewProject: *isNew, Inputs: []conventions.Input{},
		SurveyedAt: clock.Format(r.env.Clock.Now()), Conventions: digest.Bytes([]byte(doc.Conventions)), Map: digest.Bytes([]byte(doc.Map))}
	if !*isNew {
		s, err := r.lastSurvey(remote)
		if err != nil {
			return nil, err
		}
		meta.Commit, meta.Inputs = s.Commit, s.Inputs
	}
	m, _ := yaml.Marshal(meta) // strings, booleans and lists only: always encodes
	if err := r.registry().ValidateYAML(schema.Conventions, m); err != nil {
		return nil, err
	}
	dir, err := r.writeRepoFiles(remote, map[string][]byte{"conventions.md": []byte(doc.Conventions), "map.md": []byte(doc.Map), "conventions.yaml": m})
	if err != nil {
		return nil, err
	}
	return conventionsSaved{Repo: remote, Dir: dir}, nil
}

func (r *runCtx) lastSurvey(remote string) (conventions.Survey, error) {
	var s conventions.Survey
	dir, err := r.repoDir(remote)
	if err != nil {
		return s, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "survey.json")) //nolint:gosec // Athel's own survey file
	if errors.Is(err, fs.ErrNotExist) {
		return s, aerr.New(aerr.StateNotFound, "no survey of "+remote).WithNextAction("run athel repos survey " + remote)
	}
	if err != nil {
		return s, aerr.Wrap(aerr.StateIO, "read the survey", err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, aerr.Wrap(aerr.StateSchema, "read the survey", err)
	}
	return s, nil
}

type conventionsState struct {
	Meta  conventions.Meta `json:"meta"`
	Dir   string           `json:"dir"`
	Fresh bool             `json:"fresh"`
	Stale string           `json:"stale,omitempty"`
}

// conventionsOf reads a repo's conventions and judges their freshness offline, against the
// blobs at the cache clone's HEAD.
func (r *runCtx) conventionsOf(repo string) (conventionsState, error) {
	remote := r.resolveRepo(repo)
	dir, err := r.repoDir(remote)
	if err != nil {
		return conventionsState{}, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "conventions.yaml")) //nolint:gosec // Athel's own conventions record
	if errors.Is(err, fs.ErrNotExist) {
		return conventionsState{}, aerr.New(aerr.StateNotFound, "no conventions for "+remote).
			WithNextAction("run athel repos survey " + remote + ", then save conventions.md and map.md with athel conventions save")
	}
	if err != nil {
		return conventionsState{}, aerr.Wrap(aerr.StateIO, "read conventions.yaml", err)
	}
	if err := r.registry().ValidateYAML(schema.Conventions, b); err != nil {
		return conventionsState{}, err
	}
	var m conventions.Meta
	_ = yaml.Unmarshal(b, &m) // validated above
	ws, err := r.workspaceDir()
	if err != nil {
		return conventionsState{}, err
	}
	cache := repos.Cache{Dir: ws.CacheDir()}.Path(remote)
	surveyed, _ := clock.Parse(m.SurveyedAt) // conventions@1 guarantees the format
	why := conventions.Stale(m, r.env.Clock.Now(), surveyed, func(p string) (string, bool) {
		code, out, err := gitro.Run(cache, "rev-parse", "HEAD:"+p)
		return strings.TrimSpace(out), err == nil && code == 0
	})
	return conventionsState{Meta: m, Dir: dir, Fresh: why == "", Stale: why}, nil
}

func cmdConventionsShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("conventions show"), args, 1)
	if err != nil {
		return nil, err
	}
	return r.conventionsOf(pos[0])
}
