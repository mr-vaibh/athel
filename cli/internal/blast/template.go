package blast

import _ "embed"

// Template is a valid planned report with every allowed value shown, printed by
// `athel blast template` so neither the architect nor the blast-radius skill guesses the format.
// Its example comes from an unrelated domain, so it never hints at an answer.
//
//go:embed template.yaml
var Template string
