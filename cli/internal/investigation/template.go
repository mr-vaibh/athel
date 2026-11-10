package investigation

import _ "embed"

// Template is a complete, valid claim block with every allowed value listed in comments. The
// scout prints it with `athel investigation template` instead of guessing the schema. Its
// examples come from an unrelated service, so it never hints at an answer.
//
//go:embed template.yaml
var Template string
