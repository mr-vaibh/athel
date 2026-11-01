package envdetect

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/classify"
	"github.com/mr-vaibh/athel/cli/internal/config"
	"github.com/mr-vaibh/athel/cli/internal/policy"
	"github.com/mr-vaibh/athel/cli/internal/schema"
)

const envYAML = `schema: athel/environments@1
environments:
  prod:
    kube_contexts: ["gke_acme-prod_*", "*-prod-*", "arn:aws:eks:*:111111111111:cluster/*"]
    gcloud_projects: ["acme-prod", "acme-prod-data"]
    aws_accounts: ["111111111111"]
    urls: ["*.acme.com", "api.acme.com"]
  staging:
    kube_contexts: ["gke_acme-staging_*", "shared-*"]
    gcloud_projects: ["acme-staging"]
    aws_accounts: ["222222222222"]
    urls: ["*.staging.acme.com"]
  dev:
    kube_contexts: ["kind-*", "shared-*"]
default: prod
`

func resolver(t *testing.T, files map[string]string, env map[string]string) *Resolver {
	t.Helper()
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, "environments.yaml"), []byte(envYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	pol, err := policy.Load(config.Paths{Home: t.TempDir(), ProjectDir: proj}, schema.NewRegistry(), "x")
	if err != nil {
		t.Fatal(err)
	}
	return &Resolver{Policy: pol, Home: "/home/dev",
		Getenv: func(k string) string { return env[k] },
		ReadFile: func(p string) ([]byte, error) {
			if s, ok := files[p]; ok {
				return []byte(s), nil
			}
			return nil, errors.New("not found")
		}}
}

func kubeAction(h classify.EnvHints) classify.Action {
	return classify.Action{Tool: "kubectl", Remote: true, Env: h}
}

func TestResolutionOrder(t *testing.T) {
	files := map[string]string{
		"/home/dev/.kube/config": "apiVersion: v1\ncurrent-context: gke_acme-prod_eu\n",
		"/k/staging.yaml":        "current-context: gke_acme-staging_eu\n",
		"/k/empty.yaml":          "contexts: []\n",
	}
	tests := []struct {
		name string
		env  map[string]string
		h    classify.EnvHints
		want string
		how  string
	}{
		{"flag wins over everything", map[string]string{"KUBECONFIG": "/k/staging.yaml"}, classify.EnvHints{Kube: "kind-local"}, "dev", "--context"},
		{"kubeconfig in the command", nil, classify.EnvHints{KubeCfg: "/k/staging.yaml"}, "staging", "current-context"},
		{"KUBECONFIG of the hook", map[string]string{"KUBECONFIG": "/k/empty.yaml:/k/staging.yaml"}, classify.EnvHints{}, "staging", "current-context"},
		{"live current context", nil, classify.EnvHints{}, "prod", "current-context gke_acme-prod_eu"},
		{"unknown flag value", nil, classify.EnvHints{Kube: "?"}, "prod", "default (unknown kube context)"},
		{"unknown kubeconfig", nil, classify.EnvHints{KubeCfg: "?"}, "prod", "default"},
		{"unmatched context", nil, classify.EnvHints{Kube: "minikube"}, "prod", "matches no environment"},
		{"eks arn", nil, classify.EnvHints{Kube: "arn:aws:eks:eu-west-1:111111111111:cluster/main"}, "prod", "--context"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver(t, files, tt.env).Resolve(kubeAction(tt.h))
			if got.Env != tt.want || !strings.Contains(got.How, tt.how) {
				t.Errorf("got %+v, want %s (%s)", got, tt.want, tt.how)
			}
		})
	}
	noConfig := resolver(t, nil, nil).Resolve(kubeAction(classify.EnvHints{}))
	if noConfig.Env != "prod" || !strings.Contains(noConfig.How, "no current kube context") {
		t.Errorf("no kubeconfig: %+v", noConfig)
	}
}

func TestMultipleEnvsStrictest(t *testing.T) {
	r := resolver(t, nil, nil)
	if got := r.Resolve(kubeAction(classify.EnvHints{Kube: "shared-cluster"})); got.Env != "staging" {
		t.Errorf("shared context matching staging and dev = %+v", got)
	}
	if got := r.Resolve(kubeAction(classify.EnvHints{Kube: "eu-prod-1"})); got.Env != "prod" {
		t.Errorf("= %+v", got)
	}
	got := r.Resolve(classify.Action{Tool: "curl", Remote: true, Env: classify.EnvHints{URLs: []string{"https://x.staging.acme.com/a", "https://api.acme.com/b"}}})
	if got.Env != "prod" {
		t.Errorf("two URLs: %+v", got)
	}
}

func TestGcloudActiveConfig(t *testing.T) {
	files := map[string]string{
		"/home/dev/.config/gcloud/active_config":                 "work\n",
		"/home/dev/.config/gcloud/configurations/config_work":    "[core]\naccount = dev@x\nproject = acme-staging\n",
		"/home/dev/.config/gcloud/configurations/config_prodcfg": "[core]\nproject = acme-prod\n",
	}
	tests := []struct {
		env  map[string]string
		h    classify.EnvHints
		want string
	}{
		{nil, classify.EnvHints{Project: "acme-prod-data"}, "prod"},
		{map[string]string{"CLOUDSDK_CORE_PROJECT": "acme-staging"}, classify.EnvHints{}, "staging"},
		{nil, classify.EnvHints{}, "staging"},
		{map[string]string{"CLOUDSDK_ACTIVE_CONFIG_NAME": "prodcfg"}, classify.EnvHints{}, "prod"},
		{nil, classify.EnvHints{GConfig: "prodcfg"}, "prod"},
		{nil, classify.EnvHints{GConfig: "?"}, "prod"},
		{nil, classify.EnvHints{GConfig: "missing"}, "prod"},
	}
	for _, tt := range tests {
		got := resolver(t, files, tt.env).Resolve(classify.Action{Tool: "gcloud", Remote: true, Env: tt.h})
		if got.Env != tt.want {
			t.Errorf("%+v %v: got %+v, want %s", tt.h, tt.env, got, tt.want)
		}
	}
	if got := resolver(t, nil, nil).Resolve(classify.Action{Tool: "gsutil", Remote: true}); got.Env != "prod" || !strings.Contains(got.How, "no active gcloud configuration") {
		t.Errorf("no gcloud config: %+v", got)
	}
}

func TestAWSProfiles(t *testing.T) {
	files := map[string]string{"/home/dev/.aws/config": `[default]
sso_account_id = 222222222222
[profile prod-admin]
role_arn = arn:aws:iam::111111111111:role/Admin
[profile other]
region = eu-west-1
`}
	tests := []struct {
		env  map[string]string
		h    classify.EnvHints
		want string
	}{
		{nil, classify.EnvHints{}, "staging"},
		{nil, classify.EnvHints{Profile: "prod-admin"}, "prod"},
		{map[string]string{"AWS_PROFILE": "prod-admin"}, classify.EnvHints{}, "prod"},
		{nil, classify.EnvHints{Profile: "other"}, "prod"},
		{nil, classify.EnvHints{Profile: "?"}, "prod"},
		{nil, classify.EnvHints{Account: "222222222222"}, "staging"},
	}
	for _, tt := range tests {
		got := resolver(t, files, tt.env).Resolve(classify.Action{Tool: "aws", Remote: true, Env: tt.h})
		if got.Env != tt.want {
			t.Errorf("%+v %v: got %+v, want %s", tt.h, tt.env, got, tt.want)
		}
	}
}

func TestUnknownIsProdAndNotApplicable(t *testing.T) {
	r := resolver(t, nil, nil)
	tests := []struct {
		a    classify.Action
		want string
	}{
		{classify.Action{Tool: "rm"}, NotApplicable},
		{classify.Action{Tool: "git", Remote: true}, NotApplicable},
		{classify.Action{Tool: "terraform", Remote: true}, "prod"},
		{classify.Action{Tool: "raptor", Remote: true}, "prod"},
		{classify.Action{Tool: "curl", Remote: true, Env: classify.EnvHints{URLs: []string{"https://user:pw@hooks.slack.com:443/x?y"}}}, "prod"},
		// doc 04's own example patterns overlap: *.acme.com (prod) also matches staging hosts, and
		// a target in two environments is decided as the stricter one.
		{classify.Action{Tool: "WebFetch", Remote: true, Env: classify.EnvHints{URLs: []string{"https://a.staging.acme.com"}}}, "prod"},
	}
	for _, tt := range tests {
		if got := r.Resolve(tt.a); got.Env != tt.want {
			t.Errorf("%s: %+v, want %s", tt.a.Tool, got, tt.want)
		}
	}
	bare := &Resolver{}
	if got := bare.Resolve(kubeAction(classify.EnvHints{Kube: "anything"})); got.Env != "prod" {
		t.Errorf("no policy: %+v", got)
	}
}

func TestNeverReadsRealHome(t *testing.T) {
	var asked []string
	r := &Resolver{Home: "/fixture/home", ReadFile: func(p string) ([]byte, error) {
		asked = append(asked, p)
		return nil, errors.New("no")
	}}
	r.Resolve(kubeAction(classify.EnvHints{}))
	r.Resolve(classify.Action{Tool: "gcloud", Remote: true})
	r.Resolve(classify.Action{Tool: "aws", Remote: true})
	r.Resolve(kubeAction(classify.EnvHints{KubeCfg: "~/k.yaml"}))
	real, _ := os.UserHomeDir()
	for _, p := range asked {
		if !strings.HasPrefix(p, "/fixture/home/") || (real != "" && strings.HasPrefix(p, real)) {
			t.Errorf("read outside the injected home: %s", p)
		}
	}
	if len(asked) < 4 {
		t.Errorf("asked = %v", asked)
	}
}

func TestHelpers(t *testing.T) {
	cases := map[[2]string]bool{
		{"gke_acme-prod_*", "gke_acme-prod_eu"}: true, {"*-prod-*", "eu-prod-1"}: true, {"api.acme.com", "api.acme.com"}: true,
		{"*.acme.com", "acme.com"}: false, {"a?c", "abc"}: true, {"a.c", "abc"}: false, {"*", "arn:aws:x/y"}: true,
	}
	for in, want := range cases {
		if Glob(in[0], in[1]) != want {
			t.Errorf("Glob(%q, %q) != %v", in[0], in[1], want)
		}
	}
	if hostOf("api.acme.com/x") != "api.acme.com" || hostOf("https://h:8080") != "h" {
		t.Error("hostOf")
	}
	if strictness("qa") != 1 || strictness("production") != 3 || strictness("local") != 0 || strictness("stage") != 2 {
		t.Error("strictness")
	}
	if iniValue([]byte("[a]\nk=v\n[b]\nk = w\n"), "b", "k") != "w" || iniValue(nil, "a", "k") != "" {
		t.Error("iniValue")
	}
}
