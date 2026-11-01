package classify

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/mr-vaibh/athel/cli/internal/shellparse"
)

// secretGlobs are doc 04's secret file name patterns; secretDirs are credential stores.
var (
	secretGlobs   = []string{".env*", "*.pem", "*.key", "id_*", "credentials*", "*secret*", "*.p12", "*.pfx", "*.jks", ".netrc", ".pgpass", ".npmrc", ".pypirc", "*.kdbx", "*.tfvars", "*.tfstate"}
	secretAllowed = []string{".env.example", ".env.sample", ".env.template", ".env.dist", "*.pub"}
	secretDirs    = []string{"/.ssh/", "/.aws/", "/.config/gcloud/", "/.kube/", "/.docker/config.json", "/.gnupg/", "/.config/gh/", "/.azure/", "/proc/self/environ", "/proc/1/environ"}
)

// IsSecretPath reports whether reading p could expose a secret value (doc 04 §Secrets,
// FR-SAFE-007). Matching is on the base name and on known credential directories.
func IsSecretPath(p string) bool {
	if p == "" {
		return false
	}
	norm := "/" + strings.TrimPrefix(filepath.ToSlash(p), "/")
	for _, d := range secretDirs {
		if strings.Contains(norm+"/", d) || strings.HasSuffix(norm, strings.TrimSuffix(d, "/")) {
			return true
		}
	}
	if strings.Contains(norm, "/proc/") && strings.HasSuffix(norm, "/environ") {
		return true
	}
	base := path.Base(norm)
	for _, g := range secretAllowed {
		if ok, _ := path.Match(g, base); ok {
			return false
		}
	}
	for _, g := range secretGlobs {
		if ok, _ := path.Match(g, strings.ToLower(base)); ok {
			return true
		}
	}
	return false
}

// absolute resolves p against cwd; $HOME and unknown values stay symbolic for the engine.
func absolute(cwd, p string) string {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "$HOME") || cwd == "" {
		return p
	}
	return path.Join(filepath.ToSlash(cwd), p)
}

// readers print or copy file contents: on a secret path they are secret reads.
var readers = map[string]bool{
	"cat": true, "less": true, "more": true, "head": true, "tail": true, "bat": true, "grep": true, "egrep": true,
	"rg": true, "ag": true, "sed": true, "awk": true, "cut": true, "sort": true, "uniq": true, "strings": true,
	"xxd": true, "od": true, "hexdump": true, "base64": true, "nl": true, "tac": true, "jq": true, "yq": true,
	"vim": true, "vi": true, "nano": true, "emacs": true, "view": true, "open": true, "code": true, "diff": true,
	"cp": true, "scp": true, "rsync": true, "tar": true, "zip": true, "gzip": true, "source": true, ".": true,
	"openssl": true, "gpg": true,
}

// localWriters change files on this machine (W0); their non-flag arguments are targets.
var localWriters = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true, "touch": true, "mkdir": true, "chmod": true,
	"chown": true, "chgrp": true, "ln": true, "tee": true, "truncate": true, "dd": true, "install": true,
	"unlink": true, "shred": true, "patch": true, "unzip": true, "tar": true, "rsync": true, "mktemp": true,
}

// readOnly are commands with no side effects beyond printing (unless they read a secret).
var readOnly = map[string]bool{
	"ls": true, "pwd": true, "echo": true, "printf": true, "true": true, "false": true, "test": true,
	"[": true, "which": true, "type": true, "whoami": true, "id": true, "date": true, "uname": true,
	"hostname": true, "wc": true, "find": true, "fd": true, "tree": true, "stat": true, "file": true,
	"du": true, "df": true, "basename": true, "dirname": true, "realpath": true, "readlink": true,
	"sleep": true, "cd": true, "pushd": true, "popd": true, "read": true, "alias": true, "export": true,
	"seq": true, "expr": true, "tr": true, "column": true, "less": true, "man": true, "help": true,
	"ps": true, "top": true, "uptime": true, "free": true, "lsof": true, "netstat": true, "history": true,
	"shasum": true, "sha256sum": true, "md5sum": true, "cksum": true, "comm": true, "join": true, "paste": true,
	"tput": true, "clear": true, "jobs": true, "wait": true, "local": true, "return": true, "shift": true,
	"set": false, "trap": true, "umask": true, "getopts": true, "xdg-open": true, "pbcopy": true, "pbpaste": true,
}

// fileCommand classifies plain file and text commands.
func fileCommand(cmd shellparse.Command, cwd string) (Action, bool) {
	name := path.Base(cmd.Name())
	a := Action{Tool: name, Text: text(cmd), Via: cmd.Via}
	args := operands(cmd.Args[1:])
	if readers[name] || name == "cp" || name == "scp" {
		for _, p := range args {
			if !p.Known || IsSecretPath(p.Value) {
				if p.Known {
					a.Tier, a.Forbidden = X, "reads a secret file ("+p.Value+")"
					return a, true
				}
			}
		}
	}
	switch {
	case localWriters[name] || isInPlaceEdit(name, cmd.Args):
		a.Tier = W0
		for _, p := range args {
			a.Paths = append(a.Paths, pathOf(cwd, p))
		}
		return a, true
	case readers[name] || readOnly[name]:
		a.Tier = R0
		for _, p := range args {
			a.Paths = append(a.Paths, pathOf(cwd, p))
		}
		return a, true
	}
	return a, false
}

func isInPlaceEdit(name string, args []shellparse.Word) bool {
	if name != "sed" && name != "perl" {
		return false
	}
	for _, a := range args[1:] {
		if a.Known && (a.Value == "-i" || strings.HasPrefix(a.Value, "-i") || a.Value == "--in-place") {
			return true
		}
	}
	return false
}

// pathOf renders a path operand; an unknown operand becomes "?" (the engine treats it as
// outside every allowed area).
func pathOf(cwd string, w shellparse.Word) string {
	if !w.Known {
		return "?"
	}
	return absolute(cwd, w.Value)
}

// operands drops flags (words starting with "-"), keeping unknown words.
func operands(args []shellparse.Word) []shellparse.Word {
	var out []shellparse.Word
	for _, a := range args {
		if a.Known && strings.HasPrefix(a.Value, "-") && a.Value != "-" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func text(cmd shellparse.Command) string {
	parts := make([]string, 0, len(cmd.Args))
	for _, a := range cmd.Args {
		if a.Known {
			parts = append(parts, a.Value)
		} else {
			parts = append(parts, "<unknown>")
		}
	}
	return strings.Join(parts, " ")
}
