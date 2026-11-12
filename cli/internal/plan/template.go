package plan

import _ "embed"

// Template is a valid example with every allowed value listed in comments, printed by
// `athel plan template` so the architect never guesses the format. Its example comes
// from an unrelated domain, so it never hints at an answer.
//
//go:embed template.yaml
var Template string
