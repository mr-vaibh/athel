// Package athel embeds the files the helper CLI ships inside its binary: the versioned JSON
// Schemas for every state and config file. Built-in policy defaults join them in a later task.
package athel

import "embed"

// Schemas holds schemas/<name>@<version>.json, read-only.
//
//go:embed schemas/*.json
var Schemas embed.FS
