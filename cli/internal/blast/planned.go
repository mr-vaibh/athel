package blast

import (
	"bytes"
	"slices"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

// CheckPlanned lists what a planned report on a change lacks (doc 05 FR-BLAST-001/002; M3 plan
// §4.5): every changed item and data change cites the spec requirement it comes from (evidence
// kind "spec", ref REQ-n of the change), and the report names what was ruled out, since doc 05
// makes the "Not affected (checked)" list required.
func CheckPlanned(r *Report, reqs []string) []string {
	var problems []string
	if len(reqs) == 0 {
		return []string{"a planned report comes after the spec; the change has none yet"}
	}
	cites := func(ev []Evidence) bool {
		return slices.ContainsFunc(ev, func(e Evidence) bool { return e.Kind == "spec" && slices.Contains(reqs, e.Ref) })
	}
	for _, d := range r.Direct {
		if !cites(d.Evidence) {
			problems = append(problems, "changed item "+d.Item+" cites no requirement of the spec (evidence kind spec, ref REQ-n)")
		}
	}
	for _, m := range r.Data {
		if !cites(m.Evidence) {
			problems = append(problems, "data change on "+m.Table+" cites no requirement of the spec")
		}
	}
	if len(r.NotAffected) == 0 {
		problems = append(problems, "not_affected is empty; list what was checked and ruled out, and how (doc 05)")
	}
	return problems
}

// Extract reads the report block back from blast-radius.md.
func Extract(reg *schema.Registry, md []byte) (*Report, error) {
	const start, end = "```yaml\n", "\n```"
	i := bytes.LastIndex(md, []byte(start))
	if i < 0 {
		return nil, aerr.New(aerr.StateSchema, "blast-radius.md has no report block")
	}
	body := md[i+len(start):]
	if j := bytes.Index(body, []byte(end)); j >= 0 {
		body = body[:j+1]
	}
	return Decode(reg, body)
}
