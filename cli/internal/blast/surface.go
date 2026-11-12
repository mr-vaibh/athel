package blast

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// Surface is what a diff changes (doc 05 §Analysis steps, step 1, from the diff): routes, events,
// migrations, config keys, dependency manifests and exported symbols, found by deterministic
// per-language patterns (ADR 022). Files no pattern explains are listed as unclassified, which
// never lowers risk.
type Surface struct {
	Routes       []Touch  `json:"routes"`
	Events       []Touch  `json:"events"`
	Migrations   []string `json:"migrations"`
	Config       []Touch  `json:"config"`
	Dependencies []string `json:"dependencies"`
	Symbols      []Touch  `json:"symbols"`
	Unclassified []string `json:"unclassified"`
}

// Touch is one named thing a diff added or removed.
type Touch struct {
	Name string `json:"name"`
	Op   string `json:"op"` // added | removed
	File string `json:"file"`
}

var (
	goRouteRe   = regexp.MustCompile(`HandleFunc\(\s*"((?:[A-Z]+ )?/[^"]*)"`)
	tsRouteRe   = regexp.MustCompile(`\b(?:router|app|r)\.(get|post|put|patch|delete)\(\s*['"\x60](/[^'"\x60]*)`)
	eventRe     = regexp.MustCompile(`["'\x60]([a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+\.v[0-9]+)["'\x60]`)
	configRe    = regexp.MustCompile(`(?:os\.Getenv|os\.LookupEnv)\(\s*"([A-Z][A-Z0-9_]*)"|process\.env\.([A-Z][A-Z0-9_]*)`)
	goSymbolRe  = regexp.MustCompile(`^(?:func(?:\s+\([^)]*\))?|type|var|const)\s+([A-Z][A-Za-z0-9_]*)`)
	manifests   = []string{"go.mod", "package.json", "pom.xml", "build.gradle", "Cargo.toml", "pyproject.toml", "requirements.txt", "Gemfile"}
	migrationRe = regexp.MustCompile(`(^|/)(migrations?|db/migrate)/`)
)

// ParseDiff reads `git diff --unified=0` output into a surface. A changed line that moves a name
// (removed and added in the same file) counts once as added and once as removed, so a rename
// shows as both.
func ParseDiff(diff string) Surface {
	s := Surface{Routes: []Touch{}, Events: []Touch{}, Migrations: []string{}, Config: []Touch{}, Dependencies: []string{}, Symbols: []Touch{}, Unclassified: []string{}}
	explained := map[string]bool{}
	var files []string
	file := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if file == "/dev/null" {
				continue
			}
			files = appendNew(files, file)
		case strings.HasPrefix(line, "--- a/"):
			files = appendNew(files, strings.TrimPrefix(line, "--- a/"))
			if file == "" || file == "/dev/null" {
				file = strings.TrimPrefix(line, "--- a/")
			}
		case strings.HasPrefix(line, "diff --git "):
			file = ""
		case (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")) && file != "":
			op := "added"
			if line[0] == '-' {
				op = "removed"
			}
			if s.scanLine(strings.TrimSpace(line[1:]), op, file) {
				explained[file] = true
			}
		}
	}
	s.classifyFiles(files, explained)
	return s
}

// classifyFiles sorts each changed file into migrations, dependency manifests, or unclassified
// when no changed line of it explained anything.
func (s *Surface) classifyFiles(files []string, explained map[string]bool) {
	for _, f := range files {
		switch {
		case migrationRe.MatchString(f):
			s.Migrations = appendNew(s.Migrations, f)
		case slices.Contains(manifests, path.Base(f)):
			s.Dependencies = appendNew(s.Dependencies, f)
		case !explained[f]:
			s.Unclassified = appendNew(s.Unclassified, f)
		}
	}
}

// scanLine records what one changed line touches and reports whether it explained anything.
func (s *Surface) scanLine(text, op, file string) bool {
	found := false
	add := func(list *[]Touch, name string) {
		t := Touch{Name: name, Op: op, File: file}
		if !slices.Contains(*list, t) {
			*list = append(*list, t)
		}
		found = true
	}
	for _, m := range goRouteRe.FindAllStringSubmatch(text, -1) {
		add(&s.Routes, m[1])
	}
	for _, m := range tsRouteRe.FindAllStringSubmatch(text, -1) {
		add(&s.Routes, strings.ToUpper(m[1])+" "+m[2])
	}
	for _, m := range eventRe.FindAllStringSubmatch(text, -1) {
		add(&s.Events, m[1])
	}
	for _, m := range configRe.FindAllStringSubmatch(text, -1) {
		add(&s.Config, m[1]+m[2])
	}
	if strings.HasSuffix(file, ".go") {
		if m := goSymbolRe.FindStringSubmatch(text); m != nil {
			add(&s.Symbols, m[1])
		}
	}
	return found
}

func appendNew(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

// Drift lists what the actual surface changes that the planned report does not name
// (FR-BLAST-005): every route and event touched must appear in a planned changed item, every
// migration needs a planned data change, every config key a planned infra item, and every
// dependency manifest a planned dependency or infra item.
func Drift(actual Surface, planned *Report) []string {
	var items, infra []string
	for _, d := range planned.Direct {
		items = append(items, strings.ToLower(d.Item))
		if d.Kind == "dependency" {
			infra = append(infra, "dependency")
		}
	}
	for _, i := range planned.Infra {
		infra = append(infra, strings.ToLower(i.Item), i.Kind)
	}
	named := func(list []string, name string) bool {
		name = strings.ToLower(name)
		return slices.ContainsFunc(list, func(it string) bool { return strings.Contains(it, name) })
	}
	var out []string
	for _, r := range actual.Routes {
		if !named(items, routePath(r.Name)) {
			out = append(out, "route "+r.Name+" ("+r.Op+" in "+r.File+") is not in the planned report")
		}
	}
	for _, e := range actual.Events {
		if !named(items, e.Name) {
			out = append(out, "event "+e.Name+" ("+e.Op+" in "+e.File+") is not in the planned report")
		}
	}
	if len(actual.Migrations) > 0 && len(planned.Data) == 0 {
		out = append(out, "migrations "+strings.Join(actual.Migrations, ", ")+" but the planned report has no data change")
	}
	for _, c := range actual.Config {
		if !named(infra, c.Name) {
			out = append(out, "config key "+c.Name+" ("+c.Op+" in "+c.File+") is not in the planned report")
		}
	}
	for _, d := range actual.Dependencies {
		if !slices.Contains(infra, "dependency") && !slices.Contains(infra, "external_dependency") {
			out = append(out, "dependency manifest "+d+" changed but the planned report has no dependency change")
		}
	}
	return out
}

// routePath drops a route's method: "PUT /v1/users/{id}/phone" is planned as its path.
func routePath(route string) string {
	if i := strings.Index(route, " /"); i >= 0 {
		return route[i+1:]
	}
	return route
}

// RenderActual writes blast-actual-<repo>.md: what the diff changes and what the plan lacks.
func RenderActual(repo, base, head string, s Surface, drift []string) []byte {
	var b strings.Builder
	status := "matches the plan"
	if len(drift) > 0 {
		status = "drift: the plan must be updated"
	}
	b.WriteString("# Blast radius · actual · " + repo + " " + base + ".." + head + " · " + status + "\n\n")
	touches := func(title string, ts []Touch) {
		for _, t := range ts {
			b.WriteString("- " + title + " " + t.Name + " (" + t.Op + ", " + t.File + ")\n")
		}
	}
	b.WriteString("## Change surface\n")
	touches("route", s.Routes)
	touches("event", s.Events)
	touches("config", s.Config)
	touches("symbol", s.Symbols)
	for _, m := range s.Migrations {
		b.WriteString("- migration " + m + "\n")
	}
	for _, d := range s.Dependencies {
		b.WriteString("- dependency manifest " + d + "\n")
	}
	for _, u := range s.Unclassified {
		b.WriteString("- unclassified " + u + "\n")
	}
	if len(drift) > 0 {
		b.WriteString("\n## Not in the planned report\n")
		for _, d := range drift {
			b.WriteString("- " + d + "\n")
		}
	}
	return []byte(b.String())
}
