// Package athel embeds the files the helper CLI ships inside its binary: the versioned JSON
// Schemas for every state and config file, and the built-in policy defaults (config layer 1).
package athel

import "embed"

// Schemas holds schemas/<name>@<version>.json, read-only.
//
//go:embed schemas/*.json
var Schemas embed.FS

// Defaults holds plugin/policies/defaults/*.yaml, the built-in config layer.
//
//go:embed plugin/policies/defaults/*.yaml
var Defaults embed.FS
