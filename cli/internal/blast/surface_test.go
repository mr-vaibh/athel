package blast

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mr-vaibh/athel/cli/internal/schema"
	"github.com/mr-vaibh/athel/tests/fixtures/shop"
)

func diffOf(t *testing.T, bare, base, head string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", bare, "diff", "--unified=0", base, head).CombinedOutput()
	if err != nil {
		t.Fatalf("git diff: %v %s", err, out)
	}
	return string(out)
}

func plannedPhone() *Report {
	return &Report{
		Direct: []Direct{{Item: "PUT /v1/users/{id}/phone", Kind: "interface"}},
		Data:   []Migration{{Table: "users"}},
	}
}

// TestSurfaceOnPreparedBranches diffs the shop fixture's prepared branches (M3 task 08b).
func TestSurfaceOnPreparedBranches(t *testing.T) {
	l, err := shop.Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare := l.Repos["users"].Bare
	planned := ParseDiff(diffOf(t, bare, "main", "eval/phone-planned"))
	if len(planned.Routes) != 1 || planned.Routes[0].Name != "PUT /v1/users/{id}/phone" || planned.Routes[0].Op != "added" ||
		strings.Join(planned.Migrations, ",") != "migrations/002_phone_number.sql" || len(planned.Unclassified) != 0 {
		t.Fatalf("planned branch surface: %+v", planned)
	}
	if d := Drift(planned, plannedPhone()); len(d) != 0 {
		t.Errorf("the planned branch has no drift: %v", d)
	}
	extra := ParseDiff(diffOf(t, bare, "main", "eval/phone-extra"))
	got := strings.Join(Drift(extra, plannedPhone()), "; ")
	for _, want := range []string{"event users.updated.v1 (removed in events/publish.go)", "event users.updated.v2 (added in events/publish.go)"} {
		if !strings.Contains(got, want) {
			t.Errorf("drift %q lacks %q", got, want)
		}
	}
}

func TestParseDiffPatterns(t *testing.T) {
	diff := `diff --git a/web/routes.ts b/web/routes.ts
--- a/web/routes.ts
+++ b/web/routes.ts
@@ -1 +1 @@
-router.get('/v1/old', h)
+router.post("/v1/new", h)
diff --git a/cfg.go b/cfg.go
--- a/cfg.go
+++ b/cfg.go
@@ -1 +1,2 @@
+var Timeout = os.Getenv("PAY_TIMEOUT")
+func Exported() {}
diff --git a/web/env.ts b/web/env.ts
--- a/web/env.ts
+++ b/web/env.ts
@@ -0,0 +1 @@
+const k = process.env.API_KEY
diff --git a/go.mod b/go.mod
--- a/go.mod
+++ b/go.mod
@@ -1 +1 @@
+require example.com/x v1.0.0
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
+docs
diff --git a/gone.go b/gone.go
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-package gone
`
	s := ParseDiff(diff)
	names := func(ts []Touch) string {
		var out []string
		for _, x := range ts {
			out = append(out, x.Op+" "+x.Name)
		}
		return strings.Join(out, ", ")
	}
	if got := names(s.Routes); got != "removed GET /v1/old, added POST /v1/new" {
		t.Errorf("routes: %s", got)
	}
	if got := names(s.Config); got != "added PAY_TIMEOUT, added API_KEY" {
		t.Errorf("config: %s", got)
	}
	if got := names(s.Symbols); got != "added Timeout, added Exported" {
		t.Errorf("symbols: %s", got)
	}
	if strings.Join(s.Dependencies, ",") != "go.mod" || strings.Join(s.Unclassified, ",") != "README.md,gone.go" {
		t.Errorf("deps %v unclassified %v", s.Dependencies, s.Unclassified)
	}
	got := strings.Join(Drift(s, &Report{Infra: []Infra{{Item: "PAY_TIMEOUT env var", Kind: "env_var"}}}), "; ")
	for _, want := range []string{"route GET /v1/old", "config key API_KEY", "dependency manifest go.mod"} {
		if !strings.Contains(got, want) {
			t.Errorf("drift %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "PAY_TIMEOUT") {
		t.Errorf("a planned config key is no drift: %s", got)
	}
	if d := Drift(Surface{Migrations: []string{"m/1.sql"}}, &Report{}); len(d) != 1 || !strings.Contains(d[0], "no data change") {
		t.Errorf("unplanned migration: %v", d)
	}
	if d := Drift(Surface{Dependencies: []string{"go.mod"}}, &Report{Direct: []Direct{{Item: "x", Kind: "dependency"}}}); len(d) != 0 {
		t.Errorf("a planned dependency is no drift: %v", d)
	}
}

func TestRenderActualAndExtract(t *testing.T) {
	s := Surface{Routes: []Touch{{"PUT /x", "added", "a.go"}}, Events: []Touch{{"e.v.v2", "added", "e.go"}}, Config: []Touch{{"K", "added", "c.go"}},
		Symbols: []Touch{{"F", "added", "a.go"}}, Migrations: []string{"m/1.sql"}, Dependencies: []string{"go.mod"}, Unclassified: []string{"README.md"}}
	md := string(RenderActual("github.com/acme/users", "main", "f", s, []string{"event e.v.v2 is not in the planned report"}))
	for _, want := range []string{"· drift: the plan must be updated", "- route PUT /x (added, a.go)", "- migration m/1.sql", "- dependency manifest go.mod",
		"- unclassified README.md", "## Not in the planned report\n- event e.v.v2"} {
		if !strings.Contains(md, want) {
			t.Errorf("render lacks %q:\n%s", want, md)
		}
	}
	if md := string(RenderActual("r", "a", "b", Surface{}, nil)); !strings.Contains(md, "matches the plan") {
		t.Errorf("clean: %s", md)
	}
	reg := schema.NewRegistry()
	if _, err := Extract(reg, []byte("# no block")); err == nil {
		t.Error("no block")
	}
	if _, err := Extract(reg, []byte("```yaml\nschema: nope\n```\n")); err == nil {
		t.Error("bad block")
	}
}
