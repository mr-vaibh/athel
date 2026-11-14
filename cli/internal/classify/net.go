package classify

import (
	"slices"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/shellparse"
)

var dbClients = map[string]bool{"psql": true, "mysql": true, "mariadb": true, "mongosh": true, "mongo": true, "redis-cli": true,
	"clickhouse-client": true, "cqlsh": true, "sqlcmd": true, "cockroach": true, "pgcli": true, "mycli": true}

var readOnlySQL = []string{"select", "show", "explain", "describe", "desc", "with", "\\d", "\\l", "\\dt"}

// dbClient: any database access is W3 (doc 04: any database write) unless the query is given
// literally and is read-only.
func dbClient(name string, cmd shellparse.Command) Action {
	a := Action{Tool: name, Text: text(cmd), Remote: true, Tier: W3}
	_, vals := positionals(cmd.Args[1:], map[string]bool{"-c": true, "--command": true, "-e": true, "--execute": true, "--eval": true,
		"-h": true, "--host": true, "-U": true, "-u": true, "-d": true, "-p": true, "--port": true, "-f": true})
	for _, f := range []string{"-c", "--command", "-e", "--execute", "--eval"} {
		if q, ok := vals[f]; ok && q.Known {
			first := strings.ToLower(strings.TrimSpace(q.Value))
			for _, kw := range readOnlySQL {
				if strings.HasPrefix(first, kw) && !strings.Contains(first, ";") {
					a.Tier = R1
				}
			}
		}
	}
	return a
}

var netTools = map[string]bool{"curl": true, "wget": true, "http": true, "https": true, "xh": true, "httpie": true}

var curlValueFlags = map[string]bool{"-X": true, "--request": true, "-d": true, "--data": true, "--data-raw": true, "--data-binary": true,
	"--data-urlencode": true, "-F": true, "--form": true, "-T": true, "--upload-file": true, "--json": true, "-H": true, "--header": true,
	"-o": true, "--output": true, "-u": true, "--user": true, "-A": true, "-e": true, "--method": true, "--post-data": true,
	"--post-file": true, "-O": true, "--body-data": true, "-w": true, "--max-time": true, "-m": true, "--connect-timeout": true}

var bodyFlags = []string{"-d", "--data", "--data-raw", "--data-binary", "--data-urlencode", "-F", "--form", "-T",
	"--upload-file", "--json", "--post-data", "--post-file", "--body-data"}

// netCommand: GET is a remote read; any other method or a request body is a shared write.
func netCommand(name string, cmd shellparse.Command) Action {
	pos, vals := positionals(cmd.Args[1:], curlValueFlags)
	a := Action{Tool: name, Text: text(cmd), Remote: true, Tier: R1}
	for _, p := range pos {
		if v := known(p); strings.Contains(v, "://") || strings.Contains(v, ".") {
			a.Env.URLs = append(a.Env.URLs, v)
		}
	}
	if method := requestMethod(name, pos, vals); method != "" && method != "GET" && method != "HEAD" && method != "OPTIONS" {
		a.Tier, a.Verb = W2, method
	}
	return a
}

func requestMethod(name string, pos []shellparse.Word, vals map[string]shellparse.Word) string {
	method := strings.ToUpper(flagVal(vals, "-X") + flagVal(vals, "--request") + flagVal(vals, "--method"))
	for _, f := range bodyFlags {
		if _, ok := vals[f]; ok && method == "" {
			method = "POST"
		}
	}
	httpie := name == "http" || name == "https" || name == "xh" || name == "httpie"
	if httpie && len(pos) > 1 && slices.Contains([]string{"POST", "PUT", "PATCH", "DELETE"}, strings.ToUpper(known(pos[0]))) {
		method = strings.ToUpper(known(pos[0]))
	}
	return method
}

var remoteShells = map[string]bool{"ssh": true, "scp": true, "sftp": true, "rsync": true, "mosh": true, "telnet": true}

// remoteShell: running commands or copying files to another machine is W3; copying from a
// remote machine is a remote read.
func remoteShell(name string, cmd shellparse.Command) Action {
	a := Action{Tool: name, Text: text(cmd), Remote: true, Tier: W3}
	if name != "scp" && name != "rsync" {
		return a
	}
	pos, _ := positionals(cmd.Args[1:], map[string]bool{"-P": true, "-i": true, "-e": true, "-o": true, "-F": true})
	if len(pos) < 2 {
		return a
	}
	for _, src := range pos[:len(pos)-1] {
		if s := known(src); !strings.Contains(s, ":") && IsSecretPath(s) {
			a.Tier, a.Forbidden = X, "copies a secret file ("+s+")"
			return a
		}
	}
	if dst := known(pos[len(pos)-1]); !strings.Contains(dst, ":") || strings.HasPrefix(dst, "/") || strings.HasPrefix(dst, ".") {
		a.Tier = R1 // copying from a remote machine to here
	}
	return a
}

// pkgRule says which subcommands of a package tool publish (W3) and which only read (R0).
type pkgRule struct {
	publish []string
	read    []string
	unknown bool // runs an arbitrary package
}

var nodePM = pkgRule{publish: []string{"publish", "deprecate", "unpublish", "dist-tag", "owner", "token", "access"},
	read: []string{"ls", "list", "outdated", "view", "info", "why", "audit", "version", "--version", "help", "config"}}

var genericPublish = pkgRule{publish: []string{"publish", "deploy", "push", "upload", "release"}}

var pkgRules = map[string]pkgRule{
	"npm": nodePM, "pnpm": nodePM, "yarn": nodePM, "bun": nodePM,
	"npx": {unknown: true}, "pnpx": {unknown: true}, "bunx": {unknown: true}, "uvx": {unknown: true}, "pipx": {unknown: true},
	"go":    {read: []string{"env", "version", "list", "doc", "help"}},
	"cargo": {publish: []string{"publish", "yank", "owner"}},
	"twine": {publish: []string{"upload", "register"}},
	"pip":   genericPublish, "pip3": genericPublish, "uv": genericPublish, "poetry": genericPublish, "pipenv": genericPublish,
	"gradle": genericPublish, "./gradlew": genericPublish, "mvn": genericPublish, "./mvnw": genericPublish, "gem": genericPublish,
	"bundle": genericPublish, "rake": genericPublish, "dotnet": genericPublish, "swift": genericPublish, "composer": genericPublish,
	"mix": genericPublish,
	// The Go toolchain's local checkers (M4 gates, ADR 023): they read and at most rewrite files
	// in the folder they run in.
	"gofmt": {}, "goimports": {}, "staticcheck": {},
	"golangci-lint": {read: []string{"version", "help", "linters"}},
}

// packageCommand classifies build tools and package managers: installs and builds stay local
// (W0); publishing is irreversible (W3).
func packageCommand(name string, cmd shellparse.Command) (Action, bool) {
	rule, ok := pkgRules[name]
	if !ok {
		return Action{}, false
	}
	pos, _ := positionals(cmd.Args[1:], map[string]bool{"-C": true, "--prefix": true, "-w": true, "--workspace": true})
	sub := ""
	if len(pos) > 0 {
		sub = known(pos[0])
	}
	a := Action{Tool: name, Verb: sub, Text: text(cmd), Tier: W0}
	switch {
	case rule.unknown:
		a.Uncertain = UnknownCommand
	case slices.Contains(rule.publish, sub):
		a.Tier, a.Remote = W3, true
	case slices.Contains(rule.read, sub):
		a.Tier = R0
	}
	return a, true
}
