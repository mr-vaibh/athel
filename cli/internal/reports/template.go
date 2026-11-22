package reports

import _ "embed"

// Templates are valid drafts with every allowed value, one per role agent (M3 lesson: a
// template, never a guessed format). Their example domain is unrelated to any eval.
var (
	//go:embed build.yaml
	BuildTemplate []byte
	//go:embed qa.yaml
	QATemplate []byte
	//go:embed security.yaml
	SecurityTemplate []byte
)
