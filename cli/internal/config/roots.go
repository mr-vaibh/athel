package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/fsutil"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// SetUserRoots saves repos.search_roots in the user layer (ATHEL_HOME/user.yaml), keeping the
// rest of the file and its comments, upgrading a config@1 file to config@2. The result must load
// with every other layer: roots another layer set can only be narrowed. On failure the file is
// left as it was.
func SetUserRoots(p Paths, reg *schema.Registry, roots []string) error {
	path := filepath.Join(p.Home, AthelSpec.UserFile)
	old, err := os.ReadFile(path) //nolint:gosec // the user layer of Athel's own config
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return aerr.Wrap(aerr.StateIO, "read "+path, err)
	}
	out, err := withRoots(old, roots)
	if err != nil {
		return aerr.Wrap(aerr.PolicyConfigInvalid, path, err)
	}
	if err := reg.ValidateYAML(schema.Config, out); err != nil {
		return aerr.Wrap(aerr.PolicyConfigInvalid, path+" would not match "+schema.Config, err)
	}
	if err := fsutil.EnsureDir(p.Home); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(path, out); err != nil {
		return err
	}
	if _, err := Load(p, reg); err != nil {
		if exists {
			return errors.Join(err, fsutil.WriteFileAtomic(path, old))
		}
		return errors.Join(err, os.Remove(path))
	}
	return nil
}

// withRoots edits the YAML tree, so the developer's comments and layout survive.
func withRoots(data []byte, roots []string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, errors.New("the file is not a mapping")
	}
	set(top, "schema", &yaml.Node{Kind: yaml.ScalarNode, Value: schema.Config})
	list := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, r := range roots {
		list.Content = append(list.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: r})
	}
	set(child(top, "repos"), "search_roots", list)
	out, _ := yaml.Marshal(&doc) // a tree built from a parsed file and scalars: always encodes
	return out, nil
}

// child returns the mapping under key, replacing a non-mapping value and adding a missing key.
func child(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			if m.Content[i+1].Kind != yaml.MappingNode {
				m.Content[i+1] = &yaml.Node{Kind: yaml.MappingNode}
			}
			return m.Content[i+1]
		}
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
	return v
}

func set(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
}
