package cli

import (
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	athel "github.com/mr-vaibh/athel"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
)

type playbookList struct {
	Tools []string `json:"tools"`
}

func cmdPlaybookList(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("playbook list"), args, 0); err != nil {
		return nil, err
	}
	files, _ := fs.Glob(athel.Playbooks, "plugin/skills/integrations/*/SKILL.md") // a fixed, valid pattern
	tools := make([]string, 0, len(files))
	for _, f := range files {
		tools = append(tools, path.Base(path.Dir(f)))
	}
	slices.Sort(tools)
	return playbookList{Tools: tools}, nil
}

// cmdPlaybookShow prints one playbook as Markdown (raw, also with --json) for the scout.
func cmdPlaybookShow(r *runCtx, args []string) (any, error) {
	pos, err := parse(r.newFlags("playbook show"), args, 1)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(pos[0], "/.") {
		return nil, aerr.New(aerr.StateInvalid, "a playbook is named by its tool, e.g. kubectl")
	}
	b, err := fs.ReadFile(athel.Playbooks, "plugin/skills/integrations/"+pos[0]+"/SKILL.md")
	if err != nil {
		if b, err = r.projectPlaybook(pos[0]); err != nil {
			return nil, err
		}
	}
	return rawOutput(strings.TrimRight(string(b), "\n")), nil
}

// projectPlaybook serves the playbook of a tool a project registered itself (FR-DISC-008: a new
// tool is a tools.yaml entry plus an optional playbook, no code change). The path in the entry
// is relative to the project's .athel/config and may not leave it.
func (r *runCtx) projectPlaybook(id string) ([]byte, error) {
	missing := aerr.New(aerr.StateNotFound, "no playbook for "+id).WithNextAction("run athel playbook list")
	ws, err := r.workspaceDir()
	if err != nil {
		return nil, missing
	}
	pol, err := r.policySet()
	if err != nil {
		return nil, err
	}
	t := pol.Tools[id]
	if t == nil || t.Playbook == "" || !filepath.IsLocal(t.Playbook) {
		return nil, missing
	}
	b, err := readFile(filepath.Join(ws.ConfigDir(), t.Playbook))
	if err != nil {
		return nil, missing
	}
	return b, nil
}
