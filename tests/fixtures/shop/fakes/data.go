// Package fakes serves the running side of the shop fixture (doc 08 §Fixture system): fake
// kubectl, gcloud, gh and praxis CLIs and a fake MCP server for Grafana and Atlassian, all
// answering from testdata/runtime.yaml. Nothing reaches a real system. A command the fixture
// does not model fails loudly, so a scenario can never pass on invented data.
package fakes

import (
	_ "embed"

	"go.yaml.in/yaml/v3"
)

//go:embed testdata/runtime.yaml
var runtimeYAML []byte

// Data is runtime.yaml.
type Data struct {
	Kube    Kube    `yaml:"kube"`
	GCloud  GCloud  `yaml:"gcloud"`
	GitHub  GitHub  `yaml:"github"`
	Praxis  Praxis  `yaml:"praxis"`
	Grafana Grafana `yaml:"grafana"`
	Jira    Jira    `yaml:"jira"`
}

// Kube is the cluster.
type Kube struct {
	CurrentContext string                             `yaml:"current_context"`
	Contexts       map[string]struct{ Env string }    `yaml:"contexts"`
	Deployments    []Deployment                       `yaml:"deployments"`
	Routes         []struct{ Prefix, Service string } `yaml:"routes"`
	ConfigMaps     []struct {
		Name, App string
		Keys      []string
	} `yaml:"configmaps"`
	Secrets []struct{ Name, App string } `yaml:"secrets"`
	Logs    map[string][]string          `yaml:"logs"`
}

// Deployment is one workload; its image differs per environment.
type Deployment struct {
	Name     string            `yaml:"name"`
	App      string            `yaml:"app"`
	Repo     string            `yaml:"repo"`
	Replicas int               `yaml:"replicas"`
	Image    map[string]string `yaml:"image"`
}

// GCloud is the GCP projects.
type GCloud struct {
	CurrentProject string                                  `yaml:"current_project"`
	Projects       map[string]string                       `yaml:"projects"`
	Subscriptions  []struct{ Name, Topic, Project string } `yaml:"subscriptions"`
	Secrets        []string                                `yaml:"secrets"`
}

// GitHub holds open pull requests; code search reads the fixture's sources.
type GitHub struct {
	PRs []PR `yaml:"prs"`
}

// PR is one pull request.
type PR struct {
	Repo    string   `yaml:"repo" json:"-"`
	Number  int      `yaml:"number" json:"number"`
	Title   string   `yaml:"title" json:"title"`
	Author  string   `yaml:"author" json:"author"`
	Head    string   `yaml:"head" json:"headRefName"`
	Files   []string `yaml:"files" json:"files"`
	Updated string   `yaml:"updated" json:"updatedAt"`
	State   string   `yaml:"state" json:"state"`
}

// Praxis is the organisation's Praxis data.
type Praxis struct {
	Memories []struct{ Title, Content string } `yaml:"memories"`
	Claims   map[string][]string               `yaml:"claims"`
}

// Grafana is metrics, traces, alerts and logs.
type Grafana struct {
	Series []Series `yaml:"series"`
	Traces struct {
		Sampling float64 `yaml:"sampling"`
		Spans    []struct {
			Trace string   `yaml:"trace"`
			Path  []string `yaml:"path"`
			Route string   `yaml:"route"`
			Env   string   `yaml:"env"`
		} `yaml:"spans"`
	} `yaml:"traces"`
	Alerts []struct {
		Name, Service, Severity string
		Firing                  bool
	} `yaml:"alerts"`
	Logs []struct{ App, Line string } `yaml:"logs"`
}

// Series is one metric series with a rate per minute or a plain value.
type Series struct {
	Metric string            `yaml:"metric" json:"metric"`
	Labels map[string]string `yaml:"labels" json:"labels"`
	PerMin *float64          `yaml:"per_min" json:"per_min,omitempty"`
	Value  *float64          `yaml:"value" json:"value,omitempty"`
}

// Jira is tickets.
type Jira struct {
	Issues []Issue `yaml:"issues"`
}

// Issue is one ticket.
type Issue struct {
	Key       string   `yaml:"key" json:"key"`
	Summary   string   `yaml:"summary" json:"summary"`
	Status    string   `yaml:"status" json:"status"`
	Component string   `yaml:"component" json:"component"`
	Assignee  string   `yaml:"assignee" json:"assignee"`
	Comments  []string `yaml:"comments" json:"-"`
}

// Load parses the embedded runtime data.
func Load() (*Data, error) {
	var d Data
	if err := yaml.Unmarshal(runtimeYAML, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
