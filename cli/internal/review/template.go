package review

import _ "embed"

// Template is a valid review submission with every allowed value, for the reviewer agent
// (M3 lesson: a template, never a guessed format).
//
//go:embed template.yaml
var Template []byte
