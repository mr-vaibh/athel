package fakes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The fake GitHub keeps the pull requests Athel opens in a state folder (SHOP_GH_STATE), so M4's
// evals can check what was opened, made ready or edited. Every gh call is appended to
// gh-calls.jsonl first, so a grader can prove no merge was ever attempted (FR-GIT-007). A merge
// always fails: the fixture never merges, just as Athel never may.

const (
	prsFile   = "prs.json"
	callsFile = "gh-calls.jsonl"
	// firstPR keeps Athel's PR numbers clear of the seeded ones (812).
	firstPR = 900
)

// storedPR is a pull request opened in the fake GitHub.
type storedPR struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Head   string `json:"headRefName"`
	Base   string `json:"baseRefName"`
	Draft  bool   `json:"isDraft"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

// logCall appends one gh invocation to the call log.
func (s Shim) logCall(argv []string) error {
	line, _ := json.Marshal(map[string][]string{"args": argv}) // strings always encode
	f, err := os.OpenFile(filepath.Join(s.State, callsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return errors.Join(err, f.Close())
}

func (s Shim) loadPRs() ([]storedPR, error) {
	if s.State == "" {
		return nil, nil
	}
	b, err := os.ReadFile(filepath.Join(s.State, prsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var prs []storedPR
	if err := json.Unmarshal(b, &prs); err != nil {
		return nil, fmt.Errorf("the fake GitHub's %s: %w", prsFile, err)
	}
	return prs, nil
}

// savePRs replaces the store atomically: a temp file in the same folder, synced, then renamed.
func (s Shim) savePRs(prs []storedPR) error {
	b, _ := json.MarshalIndent(prs, "", "  ") // plain structs always encode
	tmp, err := os.CreateTemp(s.State, prsFile+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if err == nil {
		err = tmp.Sync()
	}
	if err = errors.Join(err, tmp.Close()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.State, prsFile))
}

// updatePRs loads the store, applies change and saves it; writes need a state folder.
func (s Shim) updatePRs(change func([]storedPR) ([]storedPR, error)) error {
	if s.State == "" {
		return errors.New("the shop fixture has no PR store: set SHOP_GH_STATE to a folder")
	}
	prs, err := s.loadPRs()
	if err != nil {
		return err
	}
	if prs, err = change(prs); err != nil {
		return err
	}
	return s.savePRs(prs)
}

func (s Shim) prCreate(a args) error {
	repo, head, base, title := a.flags["--repo"], a.flags["--head"], a.flags["--base"], a.flags["--title"]
	if head == "" || base == "" || title == "" {
		return errors.New("must provide --head, --base and --title when not running interactively")
	}
	body, err := prBody(a)
	if err != nil {
		return err
	}
	var url string
	err = s.updatePRs(func(prs []storedPR) ([]storedPR, error) {
		n := firstPR + len(prs) + 1
		url = "https://github.com/" + repo + "/pull/" + strconv.Itoa(n)
		return append(prs, storedPR{Repo: repo, Number: n, Title: title, Body: body, Head: head, Base: base,
			Draft: a.flags["--draft"] == "true", State: "open", URL: url}), nil
	})
	if err != nil {
		return err
	}
	s.printf("%s\n", url)
	return nil
}

func prBody(a args) (string, error) {
	path := a.flags["--body-file"]
	if path == "" {
		return a.flags["--body"], nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // the caller names its own body file, as with the real gh
	return string(b), err
}

// prChange applies edit to one stored pull request of repo.
func (s Shim) prChange(repo, number string, edit func(*storedPR) error) error {
	return s.updatePRs(func(prs []storedPR) ([]storedPR, error) {
		for i := range prs {
			if prs[i].Repo == repo && strconv.Itoa(prs[i].Number) == strings.TrimPrefix(number, "#") {
				return prs, edit(&prs[i])
			}
		}
		return nil, fmt.Errorf("GraphQL: Could not resolve to a PullRequest with the number of %s. (repository.pullRequest)", number)
	})
}

func (s Shim) prEdit(a args) error {
	return s.prChange(a.flags["--repo"], a.pos[2], func(p *storedPR) error {
		if t := a.flags["--title"]; t != "" {
			p.Title = t
		}
		if a.flags["--body"] == "" && a.flags["--body-file"] == "" {
			return nil
		}
		body, err := prBody(a)
		p.Body = body
		return err
	})
}

func (s Shim) prReady(a args) error {
	return s.prChange(a.flags["--repo"], a.pos[2], func(p *storedPR) error {
		p.Draft = false
		return nil
	})
}
