package investigation

import (
	"bytes"
	"strings"

	"go.yaml.in/yaml/v3"

	aerr "github.com/mr-vaibh/athel/cli/internal/errors"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const (
	blockStart = "```yaml\n"
	blockEnd   = "\n```\n"
)

// Render is investigation.md (FR-DISC-013): a heading, the scout's summary, then the claim block
// as the CLI checked it. The block is last, so readers find it without parsing the Markdown.
func Render(inv *Investigation, summary string) []byte {
	ref := inv.Change + inv.Ask
	block, _ := yaml.Marshal(inv) // strings, numbers, booleans and lists of them: always encodes
	var b bytes.Buffer
	b.WriteString("# Investigation · " + ref + " · " + inv.Type + "\n\n")
	b.WriteString("**Question:** " + inv.Question + "\n\n")
	if s := strings.TrimSpace(summary); s != "" {
		b.WriteString(s + "\n\n")
	}
	b.WriteString("## Claim block (" + SchemaID + ", checked by the athel CLI)\n\n")
	b.WriteString(blockStart)
	b.Write(bytes.TrimRight(block, "\n"))
	b.WriteString(blockEnd)
	return b.Bytes()
}

// Extract reads the claim block back from investigation.md.
func Extract(reg *schema.Registry, md []byte) (*Investigation, error) {
	start := bytes.LastIndex(md, []byte(blockStart))
	if start < 0 {
		return nil, aerr.New(aerr.StateSchema, "investigation.md has no yaml claim block")
	}
	body := md[start+len(blockStart):]
	end := bytes.Index(body, []byte(blockEnd))
	if end < 0 {
		return nil, aerr.New(aerr.StateSchema, "investigation.md has an unterminated claim block")
	}
	return Decode(reg, body[:end+1])
}
