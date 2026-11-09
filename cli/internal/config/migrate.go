package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// v1SchemaLine is the block-style schema line of a config@1 file, quoted or not, with an
// optional trailing comment.
var v1SchemaLine = regexp.MustCompile(`(?m)^schema:([ \t]*)(["']?)athel/config@1(["']?)([ \t\r]*(?:#[^\n]*)?)$`)

// MigrateLayers upgrades every athel.yaml layer Athel owns (org, user, project) from config@1
// to config@2 and returns the paths it upgraded (or would upgrade, with dryRun).
func MigrateLayers(p Paths, reg *schema.Registry, dryRun bool) ([]string, error) {
	paths := []string{filepath.Join(p.Home, "org", AthelSpec.Name), filepath.Join(p.Home, AthelSpec.UserFile)}
	if p.ProjectDir != "" {
		paths = append(paths, filepath.Join(p.ProjectDir, AthelSpec.Name))
	}
	migrated := []string{}
	for _, path := range paths {
		ok, err := migrateFile(reg, path, dryRun)
		if err != nil {
			return migrated, err
		}
		if ok {
			migrated = append(migrated, path)
		}
	}
	return migrated, nil
}

// migrateFile rewrites only the schema line, so the comments and layout a human wrote survive:
// config@2 adds optional keys, and every config@1 document is valid config@2 content.
func migrateFile(reg *schema.Registry, path string, dryRun bool) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an Athel config layer under ATHEL_HOME or the workspace
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	doc, err := schema.DecodeYAML(data)
	if err != nil {
		return false, aerr.Wrap(aerr.PolicyConfigInvalid, path+" is not valid YAML", err)
	}
	m, _ := doc.(map[string]any)
	if v, _ := m["schema"].(string); v != schema.ConfigV1 {
		return false, nil
	}
	if n := len(v1SchemaLine.FindAllIndex(data, -1)); n != 1 {
		return false, aerr.New(aerr.PolicyConfigInvalid, path+": no single top-level schema line to upgrade").
			WithNextAction("change its schema to athel/config@2 by hand; nothing else needs to change")
	}
	out := v1SchemaLine.ReplaceAll(data, []byte("schema:${1}${2}athel/config@2${3}${4}"))
	if err := reg.ValidateYAML(schema.Config, out); err != nil {
		return false, aerr.Wrap(aerr.PolicyConfigInvalid, path+" does not match "+schema.Config+" after the upgrade", err)
	}
	if dryRun {
		return true, nil
	}
	return true, fsutil.WriteFileAtomic(path, out)
}
