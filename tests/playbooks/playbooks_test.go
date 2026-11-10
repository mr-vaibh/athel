// Package playbooks checks the integration playbooks the scout reads (doc 03 §Playbook contract):
// each has the contract's sections, and every example it teaches is allowed by Athel's own policy
// in change mode, so a playbook can never send the scout into a denial.
package playbooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var sections = []string{"## When to use", "## Read queries", "## Filtering large output", "## Pitfalls", "## Environments"}

var (
	shBlock = regexp.MustCompile("(?s)```sh\n(.*?)```")
	mcpName = regexp.MustCompile("`(mcp__[a-z0-9_-]+__[A-Za-z0-9_-]+)`")
)

func playbooks(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../../plugin/skills/integrations/*/SKILL.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("no playbooks: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(filepath.Dir(f))] = string(b)
	}
	return out
}

func TestPlaybookSections(t *testing.T) {
	for name, body := range playbooks(t) {
		if !strings.HasPrefix(body, "---\ndescription: ") || !strings.Contains(body, "\nuser-invocable: false\n") {
			t.Errorf("%s: needs frontmatter with a description and user-invocable: false", name)
		}
		last := -1
		for _, s := range sections {
			i := strings.Index(body, "\n"+s+"\n")
			if i <= last {
				t.Errorf("%s: section %q missing or out of order", name, s)
			}
			last = i
		}
	}
}

type decider struct {
	t    *testing.T
	bin  string
	ws   string
	env  []string
	root string
}

func newDecider(t *testing.T) *decider {
	t.Helper()
	dir := t.TempDir()
	d := &decider{t: t, bin: filepath.Join(dir, "athel"), ws: filepath.Join(dir, "ws"), root: dir}
	if out, err := exec.Command("go", "build", "-o", d.bin, "github.com/mr-vaibh/athel/cli/cmd/athel").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	// Never this machine's real HOME, kube, gcloud or aws settings.
	d.env = []string{"HOME=" + filepath.Join(dir, "home"), "ATHEL_HOME=" + filepath.Join(dir, "athel-home"), "ATHEL_SESSION_ID=playbooks"}
	for _, args := range [][]string{{"init"}, {"change", "new", "--title", "playbooks", "--prd-text", "p"}, {"lock", "acquire", "CHG-001"}} {
		d.run(args...)
	}
	return d
}

func (d *decider) run(args ...string) map[string]any {
	d.t.Helper()
	if err := os.MkdirAll(d.ws, 0o700); err != nil {
		d.t.Fatal(err)
	}
	cmd := exec.Command(d.bin, append([]string{"--json", "--workspace", d.ws}, args...)...)
	cmd.Env = d.env
	out, err := cmd.Output()
	var env map[string]any
	if jerr := json.Unmarshal(out, &env); jerr != nil || env["ok"] != true {
		d.t.Fatalf("athel %v: %v %s", args, err, out)
	}
	data, _ := env["data"].(map[string]any)
	return data
}

func (d *decider) decide(tool string, input map[string]any) (string, string) {
	d.t.Helper()
	raw, _ := json.Marshal(input)
	v := d.run("policy", "decide", "--tool", tool, "--input", string(raw), "--cwd", d.ws, "--agent", "athel:scout")
	return v["decision"].(string), v["reason"].(string)
}

func TestPlaybookCommandsNotDenied(t *testing.T) {
	d := newDecider(t)
	checked := 0
	for name, body := range playbooks(t) {
		for _, block := range shBlock.FindAllStringSubmatch(body, -1) {
			for _, line := range strings.Split(strings.TrimSpace(block[1]), "\n") {
				if dec, why := d.decide("Bash", map[string]any{"command": line}); dec != "allow" {
					t.Errorf("%s teaches %q, which the scout would get %s: %s", name, line, dec, why)
				}
				checked++
			}
		}
		for _, m := range mcpName.FindAllStringSubmatch(body, -1) {
			if dec, why := d.decide(m[1], map[string]any{}); dec != "allow" {
				t.Errorf("%s names %s, which the scout would get %s: %s", name, m[1], dec, why)
			}
			checked++
		}
	}
	if checked < 30 {
		t.Errorf("only %d examples checked", checked)
	}
}
