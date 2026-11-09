package repos

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/gitro"
)

// Cache is Athel's own clones (doc 03 §Repo access, order 3): bare partial clones in
// <workspace>/cache, the base for every worktree Athel writes in (FR-DISC-018, M4). They are
// refreshed from the remote before every use; the developer's clones are only ever read.
type Cache struct {
	Dir string
	Git func(dir string, args ...string) (int, string, error) // git with the developer's credentials
}

// headsRefspec keeps branches current on fetch; a bare clone has no fetch refspec of its own.
const headsRefspec = "+refs/heads/*:refs/heads/*"

// Path is where the cache clone of remote (host/org/repo) lives.
func (c Cache) Path(remote string) string {
	return filepath.Join(c.Dir, strings.ReplaceAll(remote, "/", "__")+".git")
}

// Ensure clones remote from url when it is not cached yet, seeded from a local clone when one is
// given, and otherwise fetches it. It reports whether it cloned. A remote that cannot be reached
// is E-TOOL-UNREACHABLE, and a failed clone leaves nothing behind.
func (c Cache) Ensure(remote, url, reference string) (string, bool, error) {
	p := c.Path(remote)
	if _, err := os.Stat(p); err == nil {
		return p, false, c.run(c.Dir, "fetch "+remote, "-C", p, "fetch", "--prune", "origin")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", false, aerr.Wrap(aerr.StateIO, "check "+p, err)
	}
	if err := fsutil.EnsureDir(c.Dir); err != nil {
		return "", false, err
	}
	args := []string{"clone", "--bare", "--filter=blob:none", "--quiet"}
	if reference != "" {
		args = append(args, "--reference-if-able", reference, "--dissociate")
	}
	if err := c.run(c.Dir, "clone "+remote, append(args, url, p)...); err != nil {
		_ = os.RemoveAll(p) // a half-made cache clone must never be used; the clone error is reported
		return "", false, err
	}
	if err := c.run(c.Dir, "configure "+remote, "-C", p, "config", "remote.origin.fetch", headsRefspec); err != nil {
		_ = os.RemoveAll(p) // as above
		return "", false, err
	}
	return p, true, nil
}

// run runs git; a failure means the remote (or the cache) could not be used.
func (c Cache) run(dir, what string, args ...string) error {
	code, out, err := c.Git(dir, args...)
	if err == nil && code == 0 {
		return nil
	}
	detail := strings.TrimSpace(out)
	if err != nil {
		detail = err.Error()
	}
	return aerr.New(aerr.ToolUnreachable, what+": "+detail).
		WithNextAction("check that the developer can reach the repository (git credentials, VPN), then retry")
}

// CloneURL picks the URL to clone from: the local clone's own origin when it names the same
// repo, so the developer's protocol and credential setup apply, unless the URL itself carries
// user info (a token); then the canonical HTTPS URL, so no secret lands in the cache's config.
func CloneURL(remote, localClone string) string {
	raw := ""
	if localClone != "" {
		raw = originURL(filepath.Join(localClone, ".git"))
	}
	if raw != "" && gitro.NormaliseRemote(raw) == remote && !hasUserInfo(raw) {
		return raw
	}
	return "https://" + remote
}

// hasUserInfo reports a scheme URL with user info; git's scp form (git@host:org/repo) has none.
func hasUserInfo(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User != nil
}
