package blast

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Render is blast-radius.md in doc 05's shape: risk and rules fired, an impact table, the
// required "Not affected (checked)" list, gaps, the summary, then the report as a yaml block.
func Render(r *Report, summary string) []byte {
	var b bytes.Buffer
	risk := "unscored"
	if r.Risk != nil {
		risk = *r.Risk
	}
	fmt.Fprintf(&b, "# Blast radius · %s · %s · risk %s\n\n", r.Change+r.Ask, r.Mode, risk)
	fmt.Fprintf(&b, "**Proposed change:** %s\n\n", r.Proposal)
	rules := make([]string, 0, len(r.RulesFired))
	for _, f := range r.RulesFired {
		rules = append(rules, fmt.Sprintf("%s (%s: %s)", f.Rule, f.Level, f.Reason))
	}
	if len(rules) == 0 {
		rules = []string{"none"}
	}
	b.WriteString("Rules fired: " + strings.Join(rules, "; ") + "\n\n")
	b.WriteString("| Impact | Item | Evidence | Traffic |\n|---|---|---|---|\n")
	for _, d := range r.Direct {
		row(&b, "Changed ("+d.Change+")", d.Service+" "+d.Item, d.Evidence, "n/a")
	}
	for _, c := range r.Consumers {
		row(&b, "Consumer", c.Consumer+" uses "+c.Of, c.Evidence, traffic(c))
	}
	for _, m := range r.Data {
		rows := "unknown size"
		if m.Rows != nil {
			rows = fmt.Sprintf("%d rows", *m.Rows)
		}
		row(&b, "Data", fmt.Sprintf("%s %s.%s", m.Operation, m.Service, m.Table), m.Evidence, rows)
	}
	for _, i := range r.Infra {
		row(&b, "Infra", i.Kind+": "+i.Item, i.Evidence, "n/a")
	}
	for _, p := range r.People {
		if len(p.ActiveWork) > 0 {
			fmt.Fprintf(&b, "| People | %s: %s | owners %s | active |\n", p.Service, cell(strings.Join(p.ActiveWork, "; ")), cell(strings.Join(p.Owners, ", ")))
		}
	}
	b.WriteString("\nNot affected (checked):\n")
	for _, n := range r.NotAffected {
		fmt.Fprintf(&b, "- %s: %s (%s)\n", n.Item, n.Why, refs(n.Evidence))
	}
	if len(r.NotAffected) == 0 {
		b.WriteString("- nothing was ruled out\n")
	}
	if len(r.Gaps) > 0 {
		b.WriteString("\nGaps: " + strings.Join(r.Gaps, "; ") + "\n")
	}
	if s := strings.TrimSpace(summary); s != "" {
		b.WriteString("\n" + s + "\n")
	}
	block, _ := yaml.Marshal(r) // strings, numbers, booleans and lists of them: always encodes
	b.WriteString("\n## Report (" + SchemaID + ", risk computed by the athel CLI)\n\n```yaml\n")
	b.Write(bytes.TrimRight(block, "\n"))
	b.WriteString("\n```\n")
	return b.Bytes()
}

func row(b *bytes.Buffer, impact, item string, ev []Evidence, traffic string) {
	fmt.Fprintf(b, "| %s | %s | %s | %s |\n", impact, cell(item), cell(refs(ev)), cell(traffic))
}

func traffic(c Consumer) string {
	if c.Traffic == nil {
		return "not observed"
	}
	s := fmt.Sprintf("%g %s (%s, %s)", c.Traffic.Rate, strings.ReplaceAll(c.Traffic.Unit, "_", " "), c.Env, c.Traffic.Window)
	if c.Traffic.Sampling < 1 {
		s += fmt.Sprintf(", sampled at %g", c.Traffic.Sampling)
	}
	return s
}

func refs(ev []Evidence) string {
	out := make([]string, 0, len(ev))
	for _, e := range ev {
		out = append(out, e.Kind+" "+e.Ref)
	}
	return strings.Join(out, ", ")
}

// cell keeps a value inside one Markdown table cell.
func cell(s string) string { return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s) }
