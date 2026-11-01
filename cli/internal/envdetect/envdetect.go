// Package envdetect decides which environment an action targets (doc 04 §Environment
// detection, FR-SAFE-003). Resolution per call: an explicit flag in the command, then an
// environment variable set in the command, then the tool's live current setting read fresh from
// its config file (never cached, never by spawning the CLI: decision Q9), then the default
// (prod). A target matching several environments counts as the strictest.
package envdetect

import (
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/policy"
)

// NotApplicable is the environment of local actions and of tools without environments (git, gh).
const NotApplicable = "n/a"

// Resolution is the environment of one action and how it was found.
type Resolution struct {
	Env string `json:"env"`
	How string `json:"how"`
}

// Resolver resolves environments. Home, Getenv and ReadFile are injected so tests never touch
// the real home folder or the developer's cloud configuration.
type Resolver struct {
	Policy   *policy.Set
	Home     string
	Getenv   func(string) string
	ReadFile func(path string) ([]byte, error)
}

var kubeTools = map[string]bool{"kubectl": true, "oc": true, "helm": true, "flux": true}
var gcloudTools = map[string]bool{"gcloud": true, "gsutil": true, "bq": true}
var awsTools = map[string]bool{"aws": true, "eksctl": true, "cdk": true, "sam": true, "serverless": true}

// Resolve returns the environment an action targets.
func (r *Resolver) Resolve(a classify.Action) Resolution {
	if !a.Remote {
		return Resolution{Env: NotApplicable, How: "local"}
	}
	switch {
	case kubeTools[a.Tool]:
		return r.kube(a.Env)
	case gcloudTools[a.Tool]:
		return r.gcloud(a.Env)
	case awsTools[a.Tool]:
		return r.aws(a.Env)
	case len(a.Env.URLs) > 0:
		return r.urls(a.Env.URLs)
	case a.Tool == "terraform" || a.Tool == "tofu" || a.Tool == "pulumi" || a.Tool == "raptor":
		return r.fallback("the target of " + a.Tool + " cannot be read from the command")
	}
	return Resolution{Env: NotApplicable, How: a.Tool + " has no environment"}
}

func (r *Resolver) fallback(why string) Resolution {
	env := "prod"
	if r.Policy != nil && r.Policy.DefaultEnv != "" {
		env = r.Policy.DefaultEnv
	}
	return Resolution{Env: env, How: "default (" + why + ")"}
}

// match returns the strictest environment whose patterns of the given kind match value.
func (r *Resolver) match(kind, value, how string) Resolution {
	if value == "" || value == "?" {
		return r.fallback("unknown " + kind)
	}
	best, rank := "", -1
	if r.Policy != nil {
		for name, env := range r.Policy.Environments {
			for _, p := range patterns(env, kind) {
				if Glob(p, value) && Strictness(name) > rank {
					best, rank = name, Strictness(name)
				}
			}
		}
	}
	if best == "" {
		return r.fallback(kind + " " + value + " matches no environment")
	}
	return Resolution{Env: best, How: how + " " + value}
}

func patterns(env policy.Environment, kind string) []string {
	switch kind {
	case "kube context":
		return env.KubeContexts
	case "gcloud project":
		return env.GcloudProjects
	case "aws account":
		return env.AWSAccounts
	}
	return env.URLs
}

// Strictness ranks environments: prod is the strictest, dev the loosest, others in between.
func Strictness(name string) int {
	switch name {
	case "prod", "production":
		return 3
	case "dev", "development", "local":
		return 0
	case "staging", "stage":
		return 2
	}
	return 1
}

// Glob matches shell-style patterns where * also crosses "/" and ":" (cluster ARNs, URLs).
func Glob(pattern, value string) bool {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return err == nil && re.MatchString(value)
}

func (r *Resolver) read(p string) []byte {
	if r.ReadFile == nil {
		return nil
	}
	b, err := r.ReadFile(r.expand(p))
	if err != nil {
		return nil
	}
	return b
}

func (r *Resolver) expand(p string) string {
	if strings.HasPrefix(p, "$HOME") {
		return r.Home + strings.TrimPrefix(p, "$HOME")
	}
	if strings.HasPrefix(p, "~/") {
		return r.Home + p[1:]
	}
	return p
}

func (r *Resolver) getenv(k string) string {
	if r.Getenv == nil {
		return ""
	}
	return r.Getenv(k)
}

// kube: --context, else current-context from --kubeconfig / KUBECONFIG / ~/.kube/config.
func (r *Resolver) kube(h classify.EnvHints) Resolution {
	if h.Kube != "" {
		return r.match("kube context", h.Kube, "--context")
	}
	paths := []string{h.KubeCfg}
	if h.KubeCfg == "" {
		paths = strings.Split(r.getenv("KUBECONFIG"), ":")
	}
	if h.KubeCfg == "?" {
		return r.fallback("unknown kubeconfig")
	}
	if len(paths) == 1 && paths[0] == "" {
		paths = []string{"$HOME/.kube/config"}
	}
	for _, p := range paths {
		var cfg struct {
			CurrentContext string `yaml:"current-context"`
		}
		if b := r.read(p); b != nil && yaml.Unmarshal(b, &cfg) == nil && cfg.CurrentContext != "" {
			return r.match("kube context", cfg.CurrentContext, "current-context")
		}
	}
	return r.fallback("no current kube context")
}

// gcloud: --project / CLOUDSDK_CORE_PROJECT, else the active configuration's project.
func (r *Resolver) gcloud(h classify.EnvHints) Resolution {
	if h.Project != "" {
		return r.match("gcloud project", h.Project, "--project")
	}
	if p := r.getenv("CLOUDSDK_CORE_PROJECT"); p != "" {
		return r.match("gcloud project", p, "CLOUDSDK_CORE_PROJECT")
	}
	name := h.GConfig
	if name == "" {
		name = r.getenv("CLOUDSDK_ACTIVE_CONFIG_NAME")
	}
	if name == "" {
		name = strings.TrimSpace(string(r.read("$HOME/.config/gcloud/active_config")))
	}
	if name == "" || name == "?" {
		return r.fallback("no active gcloud configuration")
	}
	project := iniValue(r.read("$HOME/.config/gcloud/configurations/config_"+name), "core", "project")
	return r.match("gcloud project", project, "configuration "+name+" project")
}

var roleAccountRe = regexp.MustCompile(`arn:aws[a-z-]*:iam::([0-9]{12}):`)

// aws: the profile (--profile, AWS_PROFILE, default) mapped to its account through ~/.aws/config.
func (r *Resolver) aws(h classify.EnvHints) Resolution {
	if h.Account != "" {
		return r.match("aws account", h.Account, "account")
	}
	profile := h.Profile
	if profile == "" {
		profile = r.getenv("AWS_PROFILE")
	}
	if profile == "" {
		profile = "default"
	}
	if profile == "?" {
		return r.fallback("unknown AWS profile")
	}
	section := "profile " + profile
	if profile == "default" {
		section = "default"
	}
	cfg := r.read("$HOME/.aws/config")
	account := iniValue(cfg, section, "sso_account_id")
	if account == "" {
		if m := roleAccountRe.FindStringSubmatch(iniValue(cfg, section, "role_arn")); m != nil {
			account = m[1]
		}
	}
	return r.match("aws account", account, "profile "+profile+" account")
}

func (r *Resolver) urls(urls []string) Resolution {
	best := Resolution{}
	for _, u := range urls {
		res := r.match("url", hostOf(u), "url")
		if best.Env == "" || Strictness(res.Env) > Strictness(best.Env) {
			best = res
		}
	}
	return best
}

func hostOf(u string) string {
	if _, rest, ok := strings.Cut(u, "://"); ok {
		u = rest
	}
	host, _, _ := strings.Cut(u, "/")
	host, _, _ = strings.Cut(host, "?")
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return host
}

// iniValue reads key from [section] of a simple INI file.
func iniValue(b []byte, section, key string) string {
	current := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			current = strings.TrimSpace(line[1 : len(line)-1])
		case current == section:
			if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}
