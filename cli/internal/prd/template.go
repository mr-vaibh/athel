package prd

import _ "embed"

// ReviewTemplate is a valid PRD review with every allowed value shown, printed by
// `athel prd review template` so the architect never guesses the format (the second M3 smoke
// run spent 12 turns doing so). Its example comes from an unrelated domain, so it never hints at
// an answer.
//
//go:embed review_template.yaml
var ReviewTemplate string
