package classify

import (
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/shellparse"
)

// cloudSpec describes a CLI that reaches an environment (cluster, cloud project, account).
type cloudSpec struct {
	valueFlags map[string]bool
	kube       []string // flags selecting a kube context
	kubeCfg    []string
	project    []string
	gconfig    []string
	profile    []string
	writeTier  Tier
	verb       func(pos []shellparse.Word) string // the operative verb
	writeVerbs []string
	readVerbs  []string
}

var (
	kubeValueFlags = map[string]bool{"-n": true, "--namespace": true, "--context": true, "--kube-context": true, "--kubeconfig": true,
		"--cluster": true, "--user": true, "-l": true, "--selector": true, "-o": true, "--output": true, "-f": true, "--filename": true,
		"-c": true, "--container": true, "--field-selector": true, "--server": true, "-s": true, "--token": true, "--as": true,
		"--request-timeout": true, "--since": true, "--tail": true, "-p": true, "--set": true, "--values": true, "--version": true,
		"--timeout": true, "-k": true, "--kustomize": true, "--image": true, "--replicas": true}
	gcloudValueFlags = map[string]bool{"--project": true, "--configuration": true, "--region": true, "--zone": true, "--format": true,
		"--filter": true, "--limit": true, "--account": true, "--impersonate-service-account": true, "--secret": true, "--cluster": true,
		"--location": true, "-l": true, "--sort-by": true, "--image": true, "--service": true, "--member": true, "--role": true}
	awsValueFlags = map[string]bool{"--profile": true, "--region": true, "--output": true, "--query": true, "--endpoint-url": true,
		"--secret-id": true, "--name": true, "--cluster": true, "--service": true, "--bucket": true, "--key": true, "--function-name": true}
	iacValueFlags = map[string]bool{"-var": true, "-var-file": true, "-target": true, "-chdir": true, "--stack": true, "-s": true,
		"--profile": true, "--region": true, "--config": true, "--app": true, "-a": true}
)

func firstVerb(pos []shellparse.Word) string {
	if len(pos) == 0 {
		return ""
	}
	return known(pos[0])
}

var cloudTools = map[string]*cloudSpec{
	"kubectl": {valueFlags: kubeValueFlags, kube: []string{"--context"}, kubeCfg: []string{"--kubeconfig"}, writeTier: W3, verb: firstVerb,
		writeVerbs: []string{"exec", "cp", "port-forward", "attach", "debug", "proxy"}},
	"oc": {valueFlags: kubeValueFlags, kube: []string{"--context"}, kubeCfg: []string{"--kubeconfig"}, writeTier: W3, verb: firstVerb,
		readVerbs: []string{"get", "describe", "logs", "top", "explain", "api-resources", "version", "whoami", "status", "project", "projects"}},
	"helm": {valueFlags: kubeValueFlags, kube: []string{"--kube-context"}, kubeCfg: []string{"--kubeconfig"}, writeTier: W3, verb: firstVerb,
		writeVerbs: []string{"install", "upgrade", "uninstall", "delete", "rollback", "push", "test"},
		readVerbs:  []string{"list", "ls", "status", "get", "history", "show", "template", "search", "version", "lint", "env", "repo", "dependency"}},
	"gcloud": {valueFlags: gcloudValueFlags, project: []string{"--project"}, gconfig: []string{"--configuration"}, writeTier: W3, verb: gcloudVerb},
	"gsutil": {valueFlags: map[string]bool{"-p": true, "-o": true, "-h": true}, writeTier: W3, verb: firstVerb,
		writeVerbs: []string{"cp", "mv", "rm", "rsync", "setmeta", "acl", "iam", "mb", "rb", "defacl", "lifecycle", "versioning"},
		readVerbs:  []string{"ls", "cat", "stat", "du", "hash", "version"}},
	"bq": {valueFlags: map[string]bool{"--project_id": true, "--location": true, "--format": true}, project: []string{"--project_id"}, writeTier: W3,
		verb: firstVerb, readVerbs: []string{"ls", "show", "head", "version", "help"}},
	"aws":        {valueFlags: awsValueFlags, profile: []string{"--profile"}, writeTier: W3, verb: awsVerb},
	"eksctl":     {valueFlags: awsValueFlags, profile: []string{"--profile"}, writeTier: W3, verb: firstVerb, readVerbs: []string{"get", "info", "version", "utils"}},
	"terraform":  {valueFlags: iacValueFlags, writeTier: W3, verb: firstVerb, readVerbs: iacReads},
	"tofu":       {valueFlags: iacValueFlags, writeTier: W3, verb: firstVerb, readVerbs: iacReads},
	"pulumi":     {valueFlags: iacValueFlags, writeTier: W3, verb: firstVerb, readVerbs: append([]string{"preview", "stack", "whoami", "about", "logs"}, iacReads...)},
	"cdk":        {valueFlags: iacValueFlags, profile: []string{"--profile"}, writeTier: W3, verb: firstVerb, readVerbs: []string{"synth", "diff", "list", "ls", "doctor", "docs", "context"}},
	"serverless": {valueFlags: iacValueFlags, profile: []string{"--aws-profile"}, writeTier: W3, verb: firstVerb, readVerbs: []string{"info", "print", "logs", "package"}},
	"sam":        {valueFlags: iacValueFlags, profile: []string{"--profile"}, writeTier: W3, verb: firstVerb, readVerbs: []string{"build", "validate", "logs", "list", "local"}},
	"argocd":     {valueFlags: kubeValueFlags, writeTier: W3, verb: firstVerb, readVerbs: []string{"version", "login", "context", "app", "proj", "repo", "cluster", "account"}},
	"flux":       {valueFlags: kubeValueFlags, kube: []string{"--context"}, writeTier: W3, verb: firstVerb, readVerbs: []string{"get", "logs", "check", "version", "tree", "stats", "trace", "events", "diff", "export"}},
	"raptor":     {writeTier: W3, verb: firstVerb},
	"praxis":     {writeTier: W3, verb: praxisVerb},
	"vercel":     {writeTier: W3, verb: firstVerb, readVerbs: []string{"ls", "list", "inspect", "logs", "whoami", "env", "domains", "dns", "certs", "projects", "teams", "help"}},
	"fly":        {writeTier: W3, verb: firstVerb, readVerbs: []string{"status", "logs", "info", "list", "ls", "version", "auth", "apps", "regions", "doctor", "help"}},
	"heroku":     {writeTier: W3, verb: firstVerb, readVerbs: []string{"logs", "info", "apps", "config", "ps", "releases", "addons", "auth", "help"}},
	"firebase":   {writeTier: W3, verb: firstVerb, readVerbs: []string{"projects:list", "apps:list", "login", "help", "emulators:start", "serve"}},
	"docker":     {writeTier: W0, verb: firstVerb, writeVerbs: []string{"push"}},
	"podman":     {writeTier: W0, verb: firstVerb, writeVerbs: []string{"push"}},
}

var iacReads = []string{"plan", "show", "output", "validate", "fmt", "init", "version", "providers", "graph", "workspace", "get", "console", "state list", "state show", "login"}

// gcloud's verb is the first word that is a known verb, after its command groups.
func gcloudVerb(pos []shellparse.Word) string {
	for i, w := range pos {
		v := known(w)
		if slices.Contains(gcloudVerbs, v) {
			if i > 0 && known(pos[i-1]) == "config" {
				return "config " + v
			}
			return v
		}
	}
	return firstVerb(pos)
}

// gcloudVerbs are gcloud's verbs. Group names that read like verbs ("run" for Cloud Run) are
// deliberately absent, so `gcloud run deploy` is classified by "deploy".
var gcloudVerbs = []string{"list", "describe", "get-iam-policy", "read", "info", "version", "create", "delete", "update", "deploy",
	"set-iam-policy", "add-iam-policy-binding", "remove-iam-policy-binding", "set", "unset", "enable", "disable", "start", "stop",
	"resize", "import", "export", "publish", "rollback", "apply", "access", "add", "remove", "get-value", "login", "activate",
	"ssh", "scp", "submit", "patch", "replace", "restart", "reset", "promote", "cancel", "execute", "clone", "get-credentials", "print-access-token", "print-identity-token"}

// praxisVerb names Praxis calls for its tools.yaml allow-list (M2 plan Q10): a bare `praxis mcp`
// lists the gateway's functions, `praxis mcp <server> <fn>` invokes one under the org's cloud
// credentials, and memory, ig and duty are command groups.
func praxisVerb(pos []shellparse.Word) string {
	v := firstVerb(pos)
	switch {
	case v == "mcp" && len(pos) == 1:
		return "mcp list"
	case v == "mcp":
		return "mcp call"
	case (v == "memory" || v == "ig" || v == "duty") && len(pos) > 1:
		return v + " " + known(pos[1])
	}
	return v
}

// awsVerb is "service operation" (aws s3 has its own commands).
func awsVerb(pos []shellparse.Word) string {
	if len(pos) < 2 {
		return firstVerb(pos)
	}
	return known(pos[0]) + " " + known(pos[1])
}

// fixedSecretReads are secret-value reads forbidden regardless of tools.yaml (FR-DISC-012).
var fixedSecretReads = []struct{ tool, pattern string }{
	{"gcloud", "secrets versions access"}, {"gcloud", "print-access-token"}, {"gcloud", "print-identity-token"},
	{"aws", "secretsmanager get-secret-value"}, {"aws", "kms decrypt"}, {"aws", "ecr get-login-password"},
	{"aws", "sts get-session-token"}, {"heroku", "config:get"}, {"vercel", "env pull"},
	{"praxis", "git-credential"}, // brokers VCS tokens
}

var kubeSecretOutput = []string{"yaml", "json", "jsonpath", "go-template", "go-template-file", "template", "custom-columns", "jsonpath-file"}

func (cl *cmdClassifier) cloud(name string, cmd shellparse.Command) Action {
	spec := cloudTools[name]
	pos, vals := positionals(cmd.Args[1:], spec.valueFlags)
	verb := spec.verb(pos)
	a := Action{Tool: name, Verb: verb, Text: text(cmd), Remote: spec.writeTier != W0}
	a.Env = EnvHints{
		Kube:    hint(vals, cmd.Assigns, spec.kube, nil),
		KubeCfg: hint(vals, cmd.Assigns, spec.kubeCfg, []string{"KUBECONFIG"}),
		Project: hint(vals, cmd.Assigns, spec.project, []string{"CLOUDSDK_CORE_PROJECT", "GOOGLE_CLOUD_PROJECT"}),
		GConfig: hint(vals, cmd.Assigns, spec.gconfig, []string{"CLOUDSDK_ACTIVE_CONFIG_NAME"}),
		Profile: hint(vals, cmd.Assigns, spec.profile, []string{"AWS_PROFILE"}),
	}
	t := cl.registered(name)
	if t != nil {
		a.ToolID = t.ID
	}
	if f := cl.forbidden(name, t, pos, cmd); f != "" {
		a.Tier, a.Forbidden = X, f
		return a
	}
	a.Tier = cl.cloudTier(name, spec, t, verb, pos)
	if name == "docker" || name == "podman" {
		a.Remote = a.Tier == W3
	}
	return a
}

func (cl *cmdClassifier) registered(name string) *policy.Tool {
	if cl.pol == nil {
		return nil
	}
	for _, t := range cl.pol.Tools {
		if t.Kind == "cli" && t.Match.Command == name {
			return t
		}
	}
	return nil
}

// forbidden checks the fixed secret reads, kubectl secret dumps and tools.yaml patterns.
func (cl *cmdClassifier) forbidden(name string, t *policy.Tool, pos []shellparse.Word, cmd shellparse.Command) string {
	all := joinWords(cmd.Args[1:])
	verbFirst := joinWords(pos)
	for _, f := range fixedSecretReads {
		if f.tool == name && (strings.Contains(all, f.pattern) || strings.Contains(verbFirst, f.pattern)) {
			return "reads a secret value (" + f.pattern + ")"
		}
	}
	if why := kubeSecretDump(name, pos, cmd); why != "" {
		return why
	}
	if t == nil {
		return ""
	}
	for _, re := range t.ForbiddenRegexps() {
		if re.MatchString(verbFirst) || re.MatchString(all) {
			return "matches forbidden pattern " + re.String() + " for " + t.ID
		}
	}
	return ""
}

// kubeSecretDump: `kubectl get secret … -o yaml|json|jsonpath…` prints secret values.
func kubeSecretDump(name string, pos []shellparse.Word, cmd shellparse.Command) string {
	if (name != "kubectl" && name != "oc") || len(pos) < 2 || known(pos[0]) != "get" || !strings.HasPrefix(known(pos[1]), "secret") {
		return ""
	}
	_, vals := positionals(cmd.Args[1:], kubeValueFlags)
	out := flagVal(vals, "-o") + flagVal(vals, "--output")
	for _, o := range kubeSecretOutput {
		if strings.HasPrefix(out, o) {
			return "prints secret values (kubectl get secret -o " + out + ")"
		}
	}
	return ""
}

func (cl *cmdClassifier) cloudTier(name string, spec *cloudSpec, t *policy.Tool, verb string, pos []shellparse.Word) Tier {
	if tier, ok := fixedCloudTier(name, verb); ok {
		return tier
	}
	switch {
	case slices.Contains(spec.writeVerbs, verb):
		return spec.writeTier.max(W3)
	case slices.Contains(spec.readVerbs, verb):
		return R1
	}
	if t != nil {
		multi := joinWords(pos)
		if matchesVerb(t.Write, verb, multi) {
			return spec.writeTier
		}
		if matchesVerb(t.Read, verb, multi) {
			return R1
		}
	}
	return spec.writeTier // unknown verb of an environment-reaching tool: assume it writes
}

// fixedCloudTier holds the meanings that never depend on tools.yaml.
func fixedCloudTier(name, verb string) (Tier, bool) {
	switch {
	case name == "raptor":
		return W3, true // every Raptor call (doc 03, doc 04 tier W3)
	case name == "gcloud" && (verb == "config list" || verb == "config get-value"):
		return R0, true
	case name == "gcloud" && strings.HasPrefix(verb, "config "):
		return W0, true
	case (name == "docker" || name == "podman") && verb != "push":
		if slices.Contains([]string{"ps", "images", "logs", "inspect", "version", "info", "history", "search"}, verb) {
			return R0, true
		}
		return W0, true
	}
	return "", false
}

// matchesVerb compares a tools.yaml verb list with the verb or the leading words of the call
// ("config use-context", aws "describe-" prefixes).
func matchesVerb(list []string, verb, words string) bool {
	for _, v := range list {
		switch {
		case v == "*":
			return true
		case v == verb || strings.HasPrefix(words, v+" ") || words == v:
			return true
		case strings.HasSuffix(v, "-") && (strings.HasPrefix(verb, v) || strings.Contains(" "+verb, " "+v)):
			return true
		}
	}
	return false
}

func (t Tier) max(o Tier) Tier {
	if tierRank[o] > tierRank[t] {
		return o
	}
	return t
}
