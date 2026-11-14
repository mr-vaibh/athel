package findings

import _ "embed"

// Template is a valid draft with every allowed value, for any agent (M3 lesson: a template,
// never a guessed format).
//
//go:embed template.yaml
var Template []byte
