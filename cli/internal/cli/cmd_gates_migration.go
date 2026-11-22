package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/gates"
)

// The migration gate (doc 06, never waivable): online-safety checks on the task's new up
// migrations, a down migration for each, and up, down and up again on a throwaway PostgreSQL.
// The database is Athel's own, in a temp folder, reachable only through a Unix socket there;
// its commands are Athel's, not the repo's (ADR 023 follow-up), so they are not classified.

// pgDirs are standard PostgreSQL install folders, newest version first once sorted.
var pgDirs = []string{"/opt/homebrew/opt/postgresql@*/bin", "/usr/local/opt/postgresql@*/bin", "/usr/lib/postgresql/*/bin"}

// findPG is the folder holding initdb, pg_ctl and psql: on PATH, in ATHEL_PG_BIN, or in a
// standard install folder; "" when PostgreSQL is not installed.
func (r *runCtx) findPG() string {
	if p := lookPath(r.env.Getenv("PATH"), "initdb"); p != "" {
		return filepath.Dir(p)
	}
	candidates := []string{r.env.Getenv("ATHEL_PG_BIN")}
	for _, pattern := range pgDirs {
		found, _ := filepath.Glob(pattern) // the patterns are valid
		sort.Sort(sort.Reverse(sort.StringSlice(found)))
		candidates = append(candidates, found...)
	}
	for _, dir := range candidates {
		if dir != "" && lookPath(dir, "initdb") != "" && lookPath(dir, "pg_ctl") != "" && lookPath(dir, "psql") != "" {
			return dir
		}
	}
	return ""
}

// pgServer is a throwaway PostgreSQL.
type pgServer struct {
	bin, dir string
	env      []string
}

func (p *pgServer) run(name string, args ...string) (string, error) {
	cmd := exec.Command(filepath.Join(p.bin, name), args...) //nolint:gosec // Athel's own PostgreSQL binaries, fixed arguments
	cmd.Env = p.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// startPG makes a cluster in a temp folder and starts it on a Unix socket there, with no TCP.
func (r *runCtx) startPG(bin string) (*pgServer, error) {
	dir, err := os.MkdirTemp("", "athel-pg-")
	if err != nil {
		return nil, err
	}
	p := &pgServer{bin: bin, dir: dir, env: []string{"PATH=" + bin + string(os.PathListSeparator) + r.env.Getenv("PATH"), "LC_ALL=C", "LANG=C"}}
	if out, err := p.run("initdb", "-D", filepath.Join(dir, "data"), "-U", "athel", "--auth=trust", "-E", "UTF8", "--no-sync"); err != nil {
		p.stop()
		return nil, errors.New("initdb: " + strings.TrimSpace(out))
	}
	opts := "-c listen_addresses='' -k " + dir + " -c fsync=off"
	if out, err := p.run("pg_ctl", "-D", filepath.Join(dir, "data"), "-o", opts, "-w", "-l", filepath.Join(dir, "log"), "start"); err != nil {
		p.stop()
		return nil, errors.New("pg_ctl start: " + strings.TrimSpace(out))
	}
	return p, nil
}

// psql runs one SQL file, stopping at the first error.
func (p *pgServer) psql(file string) (string, error) {
	return p.run("psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-h", p.dir, "-U", "athel", "-d", "postgres", "-f", file)
}

// stop stops the server, if it runs, and removes its folder.
func (p *pgServer) stop() {
	_, _ = p.run("pg_ctl", "-D", filepath.Join(p.dir, "data"), "-m", "immediate", "stop") // best effort: the folder goes next
	_ = os.RemoveAll(p.dir)                                                               // a throwaway folder
}

// judgeMigration checks the task's new or changed migrations.
func (r *runCtx) judgeMigration(g gateTask, base *gateBaseline, o *gateOutcome) {
	names, err := r.gitOut(g.dir, "diff", "--name-only", "--diff-filter=AM", base.Head)
	if err != nil {
		o.Status, o.Summary = "unavailable", err.Error()
		return
	}
	var ups []string
	for _, n := range strings.Fields(names) {
		if gates.IsMigration(n) && !strings.HasSuffix(n, ".down.sql") {
			ups = append(ups, n)
		}
	}
	if len(ups) == 0 {
		o.Status, o.Summary = gates.Skipped, "no migration in this task"
		return
	}
	sort.Strings(ups)
	if o.Details = r.staticMigrationChecks(g, ups); len(o.Details) > 0 {
		o.Status, o.Summary = gates.Fail, "migrations that are not safe to run online, or cannot be undone"
		return
	}
	bin := r.findPG()
	if bin == "" {
		o.Status, o.Summary = "unavailable", "PostgreSQL is not installed: set ATHEL_PG_BIN to the folder with initdb, pg_ctl and psql"
		return
	}
	if why := r.runMigrations(g, bin, ups); why != "" {
		o.Status, o.Summary, o.Details = gates.Fail, "the migrations do not run up, down and up again on a fresh database", []string{why}
	}
}

// staticMigrationChecks reads each new up migration: online safety, and a down migration.
func (r *runCtx) staticMigrationChecks(g gateTask, ups []string) []string {
	var out []string
	for _, up := range ups {
		sql, err := os.ReadFile(filepath.Join(g.dir, up)) //nolint:gosec // a changed file of the worktree
		if err != nil {
			out = append(out, up+": "+err.Error())
			continue
		}
		for _, f := range gates.OnlineSafety(string(sql), g.task.Kind == "contract") {
			out = append(out, up+": "+f)
		}
		down := gates.DownFor(up)
		if _, err := os.Stat(filepath.Join(g.dir, down)); down == "" || err != nil {
			out = append(out, up+" has no down migration: name it NNN_name.up.sql with a matching NNN_name.down.sql")
		}
	}
	return out
}

// runMigrations applies every up migration of the folders involved in name order, then the
// task's downs in reverse, then its ups again; it returns the first failure, or "".
func (r *runCtx) runMigrations(g gateTask, bin string, ups []string) string {
	pg, err := r.startPG(bin)
	if err != nil {
		return err.Error()
	}
	defer pg.stop()
	var all []string
	for _, dir := range dirsOf(ups) {
		files, _ := filepath.Glob(filepath.Join(g.dir, dir, "*.sql")) // the pattern is valid
		for _, f := range files {
			if !strings.HasSuffix(f, ".down.sql") {
				all = append(all, f)
			}
		}
	}
	sort.Strings(all)
	steps := slices.Clone(all)
	for i := len(ups) - 1; i >= 0; i-- {
		steps = append(steps, filepath.Join(g.dir, gates.DownFor(ups[i])))
	}
	for _, up := range ups {
		steps = append(steps, filepath.Join(g.dir, up))
	}
	for _, f := range steps {
		if out, err := pg.psql(f); err != nil {
			rel, _ := filepath.Rel(g.dir, f) // f is under the worktree
			return rel + ": " + strings.TrimSpace(out)
		}
	}
	return ""
}

func dirsOf(files []string) []string {
	var out []string
	for _, f := range files {
		if d := filepath.Dir(f); !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}
