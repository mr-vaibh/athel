package cli

import (
	"errors"
	"os"
	"path/filepath"
	"slices"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

type layerRef struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type configValidation struct {
	Valid         bool       `json:"valid"`
	EffectiveHash string     `json:"effective_hash"`
	Layers        []layerRef `json:"layers"`
	Notes         []string   `json:"notes"`
}

func cmdConfigValidate(r *runCtx, args []string) (any, error) {
	if _, err := parse(r.newFlags("config validate"), args, 0); err != nil {
		return nil, err
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	out := configValidation{Valid: true, EffectiveHash: cfg.Hash, Notes: cfg.Notes}
	for _, l := range cfg.Layers {
		out.Layers = append(out.Layers, layerRef{Name: l.Name, Path: l.Path})
	}
	return out, nil
}

type configView struct {
	Values        map[string]any    `json:"values"`
	Origins       map[string]string `json:"origins,omitempty"`
	Overridable   []string          `json:"overridable,omitempty"`
	EffectiveHash string            `json:"effective_hash,omitempty"`
	Layer         *layerRef         `json:"layer,omitempty"`
}

func cmdConfigShow(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("config show")
	fs.Bool("effective", true, "show the merged configuration (default)")
	layer := fs.String("layer", "", "show one layer as written: builtin | org | user | project")
	if _, err := parse(fs, args, 0); err != nil {
		return nil, err
	}
	cfg, err := r.config()
	if err != nil {
		return nil, err
	}
	if *layer == "" {
		return configView{Values: cfg.Effective.Values, Origins: cfg.Effective.Origins,
			Overridable: cfg.Effective.Overridable, EffectiveHash: cfg.Hash}, nil
	}
	known := []string{"builtin", "org", "user", "project"}
	if !slices.Contains(known, *layer) {
		return nil, usagef("config show: --layer must be one of builtin, org, user, project")
	}
	for _, l := range cfg.Layers {
		if l.Name == *layer {
			return configView{Values: l.Doc, Layer: &layerRef{Name: l.Name, Path: l.Path}}, nil
		}
	}
	return nil, aerr.New(aerr.StateNotFound, "the "+*layer+" layer has no config file")
}

type schemaResult struct {
	File   string `json:"file"`
	Schema string `json:"schema"`
	Valid  bool   `json:"valid"`
}

func cmdSchemaValidate(r *runCtx, args []string) (any, error) {
	fs := r.newFlags("schema validate")
	id := fs.String("schema", "", "schema ID, e.g. athel/change@1 (default: the file's schema field)")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return nil, err
	}
	path := pos[0]
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.root(), path)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the user names the file to validate; it is only read
	if err != nil {
		code := aerr.StateIO
		if errors.Is(err, os.ErrNotExist) {
			code = aerr.StateNotFound
		}
		return nil, aerr.Wrap(code, "read "+pos[0], err)
	}
	doc, err := schema.DecodeYAML(data)
	if err != nil {
		return nil, err
	}
	if *id == "" {
		if *id, err = schema.DetectID(doc); err != nil {
			return nil, err
		}
	}
	if err := r.registry().Validate(*id, doc); err != nil {
		return nil, err
	}
	return schemaResult{File: pos[0], Schema: *id, Valid: true}, nil
}
