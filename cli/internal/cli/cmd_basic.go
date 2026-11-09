package cli

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	athel "github.com/mr-vaibh/athel"
	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/state"
)

// MinClaudeCodeVersion reads metadata.athel.min_claude_code_version from the embedded
// plugin manifest (plugin.json has no native field for it; M0-plan §3 D1).
func MinClaudeCodeVersion(manifest []byte) (string, error) {
	var m struct {
		Metadata struct {
			Athel struct {
				MinClaudeCode string `json:"min_claude_code_version"`
			} `json:"athel"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return "", aerr.Wrap(aerr.StateInvalid, "read plugin manifest", err)
	}
	if m.Metadata.Athel.MinClaudeCode == "" {
		return "", aerr.New(aerr.StateInvalid, "plugin manifest has no metadata.athel.min_claude_code_version")
	}
	return m.Metadata.Athel.MinClaudeCode, nil
}

type versionInfo struct {
	Version       string   `json:"version"`
	Schemas       []string `json:"schemas"`
	MinClaudeCode string   `json:"min_claude_code_version"`
}

func cmdVersion(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("version"), args, 0); err != nil {
		return nil, err
	}
	ids, err := r.registry().IDs()
	if err != nil {
		return nil, err
	}
	minCC, err := MinClaudeCodeVersion(athel.PluginManifest)
	if err != nil {
		return nil, err
	}
	return versionInfo{Version: r.env.Version, Schemas: ids, MinClaudeCode: minCC}, nil
}

const projectConfigTemplate = `# Project layer of the Athel configuration (layer 4 of 4).
# A project may only tighten the built-in, org and user layers unless they list a key under
# "overridable". The keys are defined by schemas/config@2.json.
schema: athel/config@2
`

type initResult struct {
	Workspace string   `json:"workspace"`
	Created   []string `json:"created"`
}

func cmdInit(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("init"), args, 0); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(r.root())
	if err != nil {
		return nil, aerr.Wrap(aerr.StateIO, "resolve workspace root", err)
	}
	ws, created, err := state.Init(root)
	if err != nil {
		return nil, err
	}
	cfgFile := filepath.Join(ws.ConfigDir(), "athel.yaml")
	if _, err := os.Stat(cfgFile); errors.Is(err, fs.ErrNotExist) {
		if err := fsutil.WriteFileAtomic(cfgFile, []byte(projectConfigTemplate)); err != nil {
			return nil, err
		}
		created = append(created, cfgFile)
	}
	if created == nil {
		created = []string{}
	}
	return initResult{Workspace: ws.Root, Created: created}, nil
}

// readPRD returns the PRD content and its source from exactly one of the three inputs.
func readPRD(r *runCtx, file, text string, stdin bool) ([]byte, string, error) {
	which, err := oneOf([]string{"--prd-file", "--prd-text", "--prd-stdin"}, []bool{file != "", text != "", stdin})
	if err != nil {
		return nil, "", err
	}
	switch which {
	case "--prd-file":
		b, err := readFile(file)
		if err != nil {
			return nil, "", aerr.Wrap(aerr.StateIO, "read PRD file "+file, err)
		}
		return b, "file", nil
	case "--prd-text":
		return []byte(text), "text", nil
	}
	b, err := io.ReadAll(r.env.Stdin)
	if err != nil {
		return nil, "", aerr.Wrap(aerr.StateIO, "read PRD from stdin", err)
	}
	return b, "stdin", nil
}

func itoa(n int) string { return strconv.Itoa(n) }
