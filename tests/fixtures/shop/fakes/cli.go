package fakes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Shim is the fake CLIs: one binary installed as kubectl, gcloud, gh and praxis on a
// scenario-only PATH.
type Shim struct {
	Data    *Data
	Sources string // the fixture's source trees, searched by `gh search code`
	Out     io.Writer
	Err     io.Writer
}

// args splits a command line into positionals and flag values.
type args struct {
	pos   []string
	flags map[string]string
}

// valueFlags take the next word as their value.
var valueFlags = map[string]bool{"--context": true, "-n": true, "--namespace": true, "-l": true, "-o": true, "--since": true,
	"--tail": true, "--field-selector": true, "--project": true, "--filter": true, "--format": true, "--repo": true,
	"--state": true, "--json": true, "--owner": true, "--limit": true, "--jq": true, "--region": true, "--git": true, "--agent": true}

func parseArgs(words []string) args {
	a := args{flags: map[string]string{}}
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case strings.HasPrefix(w, "-") && strings.Contains(w, "="):
			k, v, _ := strings.Cut(w, "=")
			a.flags[k] = v
		case valueFlags[w] && i+1 < len(words):
			a.flags[w] = words[i+1]
			i++
		case strings.HasPrefix(w, "-"):
			a.flags[w] = "true"
		default:
			a.pos = append(a.pos, w)
		}
	}
	return a
}

// Run runs one fake command; argv[0]'s base name says which CLI it is.
func (s Shim) Run(argv []string) int {
	a := parseArgs(argv[1:])
	var err error
	switch filepath.Base(argv[0]) {
	case "kubectl":
		err = s.kubectl(a)
	case "gcloud":
		err = s.gcloud(a)
	case "gh":
		err = s.gh(a)
	case "praxis":
		err = s.praxis(a)
	default:
		err = fmt.Errorf("shopctl is installed as kubectl, gcloud, gh or praxis, not %s", filepath.Base(argv[0]))
	}
	if err != nil {
		_, _ = fmt.Fprintln(s.Err, "error: "+err.Error())
		return 1
	}
	return 0
}

func (s Shim) printf(format string, v ...any) { _, _ = fmt.Fprintf(s.Out, format, v...) }

func (s Shim) json(v any) error {
	b, _ := json.MarshalIndent(v, "", "  ") // fixture values: always encode
	s.printf("%s\n", b)
	return nil
}

func unmodelled(cli string, a args) error {
	return fmt.Errorf("the shop fixture does not model `%s %s`", cli, strings.Join(a.pos, " "))
}

// --- kubectl ---------------------------------------------------------------------------------

func (s Shim) kubectl(a args) error {
	ctx := a.flags["--context"]
	if ctx == "" {
		ctx = s.Data.Kube.CurrentContext
	}
	env, ok := s.Data.Kube.Contexts[ctx]
	if !ok {
		return fmt.Errorf("context %q does not exist", ctx)
	}
	if s.kubectlInfo(strings.Join(head(a.pos, 2), " ")) {
		return nil
	}
	switch {
	case len(a.pos) >= 2 && a.pos[0] == "get" && isDeploy(a.pos[1]):
		return s.getDeploy(a, env.Env)
	case len(a.pos) >= 2 && a.pos[0] == "describe" && isDeploy(a.pos[1]):
		return s.describe(a, env.Env)
	case len(a.pos) >= 2 && a.pos[0] == "get":
		return s.getOther(a)
	case len(a.pos) == 2 && a.pos[0] == "logs":
		return s.logs(strings.TrimPrefix(a.pos[1], "deploy/"))
	default:
		return unmodelled("kubectl", a)
	}
}

// kubectlInfo answers the commands about the client and its configuration, not the cluster.
func (s Shim) kubectlInfo(cmd string) bool {
	switch cmd {
	case "auth can-i":
		s.printf("yes\n")
	case "version":
		s.printf("Client Version: v1.31.0\nServer Version: v1.30.4-gke.1\n")
	case "config current-context":
		s.printf("%s\n", s.Data.Kube.CurrentContext)
	case "config get-contexts":
		s.printf("%s\n", strings.Join(keys(s.Data.Kube.Contexts), "\n"))
	default:
		return false
	}
	return true
}

func isDeploy(kind string) bool {
	return kind == "deploy" || kind == "deployment" || kind == "deployments"
}

func (s Shim) deployments(a args) []Deployment {
	var out []Deployment
	for _, d := range s.Data.Kube.Deployments {
		if (len(a.pos) < 3 || a.pos[2] == d.Name) && (a.flags["-l"] == "" || a.flags["-l"] == "app="+d.App) {
			out = append(out, d)
		}
	}
	return out
}

func (s Shim) getDeploy(a args, env string) error {
	ds := s.deployments(a)
	if len(ds) == 0 {
		s.printf("No resources found.\n")
		return nil
	}
	if strings.Contains(a.flags["-o"], "image") {
		for _, d := range ds {
			s.printf("%s\n", d.Image[env])
		}
		return nil
	}
	s.printf("NAME READY IMAGES\n")
	for _, d := range ds {
		s.printf("%s %d/%d %s\n", d.Name, d.Replicas, d.Replicas, d.Image[env])
	}
	return nil
}

func (s Shim) describe(a args, env string) error {
	ds := s.deployments(a)
	if len(ds) == 0 {
		return fmt.Errorf("deployments.apps %q not found", strings.Join(a.pos[2:], " "))
	}
	d := ds[0]
	s.printf("Name: %s\nLabels: app=%s\nAnnotations: source-repo=%s\nReplicas: %d desired | %d available\nImage: %s\n",
		d.Name, d.App, d.Repo, d.Replicas, d.Replicas, d.Image[env])
	// The pod template's config and secret mounts, as a real describe shows them.
	for _, c := range s.Data.Kube.ConfigMaps {
		if c.App == d.App {
			s.printf("Environment Variables from:\n  %s  ConfigMap  Optional: false\n", c.Name)
		}
	}
	for _, sec := range s.Data.Kube.Secrets {
		if sec.App == d.App {
			s.printf("Mounts:\n  /var/run/secrets/%s from %s (ro)\nVolumes:\n  %s:\n    Type: Secret\n    SecretName: %s\n", sec.Name, sec.Name, sec.Name, sec.Name)
		}
	}
	return nil
}

func (s Shim) getOther(a args) error {
	kind := a.pos[1]
	switch {
	case strings.HasPrefix(kind, "configmap"):
		s.configMaps(a)
	case strings.HasPrefix(kind, "secret"):
		return s.secrets(a)
	case strings.HasPrefix(kind, "svc") || strings.HasPrefix(kind, "service"):
		for _, d := range s.deployments(args{flags: a.flags}) {
			s.printf("service/%s ClusterIP 8080\n", d.Name)
		}
	default:
		s.printf("No resources found.\n")
	}
	return nil
}

func (s Shim) configMaps(a args) {
	if len(a.pos) == 3 && a.pos[2] == "gateway-routes" {
		for _, r := range s.Data.Kube.Routes {
			s.printf("%s -> %s:8080\n", r.Prefix, r.Service)
		}
		return
	}
	for _, c := range s.Data.Kube.ConfigMaps {
		if a.flags["-l"] == "" || a.flags["-l"] == "app="+c.App {
			s.printf("%s keys=%s\n", c.Name, strings.Join(c.Keys, ","))
		}
	}
	s.printf("gateway-routes keys=routes\n")
}

func (s Shim) secrets(a args) error {
	if a.flags["-o"] != "" {
		return fmt.Errorf("the shop fixture never prints secret values")
	}
	for _, c := range s.Data.Kube.Secrets {
		if a.flags["-l"] == "" || a.flags["-l"] == "app="+c.App {
			s.printf("%s Opaque\n", c.Name)
		}
	}
	return nil
}

func (s Shim) logs(app string) error {
	lines, ok := s.Data.Kube.Logs[app]
	if !ok {
		return fmt.Errorf("deployments.apps %q has no logs in the fixture", app)
	}
	s.printf("%s\n", strings.Join(lines, "\n"))
	return nil
}

// --- gcloud ----------------------------------------------------------------------------------

func (s Shim) gcloud(a args) error {
	project := a.flags["--project"]
	if project == "" {
		project = s.Data.GCloud.CurrentProject
	}
	if _, ok := s.Data.GCloud.Projects[project]; !ok {
		return fmt.Errorf("project %q is not in the fixture (pass --project acme-prod or acme-staging)", project)
	}
	switch strings.Join(head(a.pos, 3), " ") {
	case "config get-value project":
		s.printf("%s\n", s.Data.GCloud.CurrentProject)
		return nil
	case "auth list":
		s.printf("ACTIVE  ACCOUNT\n*       dev@example.invalid\n")
		return nil
	case "pubsub subscriptions list":
		var out []map[string]string
		for _, sub := range s.Data.GCloud.Subscriptions {
			if sub.Project == project && (a.flags["--filter"] == "" || strings.Contains(a.flags["--filter"], sub.Topic)) {
				out = append(out, map[string]string{"name": "projects/" + project + "/subscriptions/" + sub.Name, "topic": "projects/" + project + "/topics/" + sub.Topic})
			}
		}
		return s.json(out)
	case "pubsub topics list":
		return s.json([]map[string]string{{"name": "projects/" + project + "/topics/users.updated.v1"}})
	case "secrets list":
		s.printf("%s\n", strings.Join(s.Data.GCloud.Secrets, "\n"))
		return nil
	case "run services list":
		return s.json([]any{})
	}
	return unmodelled("gcloud", a)
}

// --- gh --------------------------------------------------------------------------------------

func (s Shim) gh(a args) error {
	cmd := strings.Join(head(a.pos, 2), " ")
	switch {
	case cmd == "auth status":
		s.printf("github.com\n  ✓ Logged in to github.com account dev (keyring)\n")
		return nil
	case cmd == "pr list":
		return s.prList(a.flags["--repo"])
	case cmd == "pr view" && len(a.pos) == 3:
		return s.prView(a.flags["--repo"], a.pos[2])
	case cmd == "search prs":
		return s.searchPRs(strings.Join(a.pos[2:], " ") + " " + a.flags["--repo"])
	case cmd == "search code" && len(a.pos) >= 3:
		return s.searchCode(a.pos[2])
	case cmd == "run list":
		return s.json([]any{})
	case len(a.pos) == 2 && a.pos[0] == "api" && strings.HasPrefix(a.pos[1], "repos/acme/"):
		return s.api(strings.TrimPrefix(a.pos[1], "repos/acme/"))
	}
	return unmodelled("gh", a)
}

// prView shows one pull request of a repo by number, or fails as gh does.
func (s Shim) prView(repo, number string) error {
	for _, p := range s.Data.GitHub.PRs {
		if p.Repo == repo && strconv.Itoa(p.Number) == strings.TrimPrefix(number, "#") {
			return s.json(p)
		}
	}
	return fmt.Errorf("GraphQL: Could not resolve to a PullRequest with the number of %s. (repository.pullRequest)", number)
}

// searchPRs returns open pull requests whose repo, title, branch or files mention a query word.
func (s Shim) searchPRs(query string) error {
	out := []PR{}
	words := strings.Fields(strings.ToLower(query))
	for _, p := range s.Data.GitHub.PRs {
		text := strings.ToLower(p.Repo + " " + p.Title + " " + p.Head + " " + strings.Join(p.Files, " "))
		if p.State == "open" && slices.ContainsFunc(words, func(w string) bool { return strings.Contains(text, w) }) {
			out = append(out, p)
		}
	}
	return s.json(out)
}

func (s Shim) prList(repo string) error {
	out := []PR{}
	for _, p := range s.Data.GitHub.PRs {
		if p.Repo == repo && p.State == "open" {
			out = append(out, p)
		}
	}
	return s.json(out)
}

type codeHit struct {
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Path string `json:"path"`
}

func (s Shim) searchCode(query string) error {
	hits := []codeHit{}
	err := filepath.WalkDir(s.Sources, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) //nolint:gosec // the fixture's own sources
		if err != nil || !strings.Contains(string(b), query) {
			return err
		}
		rel, _ := filepath.Rel(s.Sources, p) // p is under Sources
		repo, path, _ := strings.Cut(filepath.ToSlash(rel), "/")
		h := codeHit{Path: path}
		h.Repository.NameWithOwner = "acme/" + repo
		hits = append(hits, h)
		return nil
	})
	if err != nil {
		return err
	}
	return s.json(hits)
}

func (s Shim) api(rest string) error {
	repo, path, _ := strings.Cut(rest, "/")
	switch {
	case path == "branches":
		branches := []map[string]string{{"name": "main"}}
		if repo == "users" {
			branches = append(branches, map[string]string{"name": "teammate/profile-phone"})
		}
		return s.json(branches)
	case strings.HasPrefix(path, "contents/"):
		b, err := os.ReadFile(filepath.Join(s.Sources, repo, filepath.FromSlash(strings.TrimPrefix(path, "contents/")))) //nolint:gosec // the fixture's own sources
		if err != nil {
			return fmt.Errorf("HTTP 404: Not Found (repos/acme/%s)", rest)
		}
		return s.json(map[string]string{"path": strings.TrimPrefix(path, "contents/"), "encoding": "base64", "content": base64.StdEncoding.EncodeToString(b)})
	}
	return fmt.Errorf("the shop fixture does not model `gh api repos/acme/%s`", rest)
}

// --- praxis ----------------------------------------------------------------------------------

func (s Shim) praxis(a args) error {
	switch strings.Join(head(a.pos, 2), " ") {
	case "status":
		return s.json(map[string]any{"profile": "shop-fixture", "authenticated": true})
	case "memory recall":
		return s.json(s.recall(strings.Join(a.pos[2:], " ")))
	case "memory list":
		return s.json(s.Data.Praxis.Memories)
	case "ig claims":
		return s.json(s.Data.Praxis.Claims[strings.TrimSuffix(strings.TrimPrefix(a.flags["--git"], "https://"), ".git")])
	case "duty list", "duty findings", "mcp":
		return s.json([]any{})
	}
	return unmodelled("praxis", a)
}

func (s Shim) recall(query string) []map[string]string {
	out := []map[string]string{}
	for _, m := range s.Data.Praxis.Memories {
		for _, w := range strings.Fields(strings.ToLower(query)) {
			if strings.Contains(strings.ToLower(m.Title+" "+m.Content), w) {
				out = append(out, map[string]string{"title": m.Title, "content": m.Content})
				break
			}
		}
	}
	return out
}

func head(list []string, n int) []string { return list[:min(n, len(list))] }

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
