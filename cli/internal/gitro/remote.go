package gitro

import (
	"net/url"
	"regexp"
	"strings"
)

// scpLike is git's user@host:path form.
var scpLike = regexp.MustCompile(`^[A-Za-z0-9._-]+@([A-Za-z0-9.-]+):(.+)$`)

// defaultPorts are left out of a normalised remote, so the same repo has one name.
var defaultPorts = map[string]string{"ssh": "22", "https": "443", "http": "80", "git": "9418"}

// NormaliseRemote turns a git remote URL into host/org/repo (doc 03 §Finding local clones, step
// 3): SSH and HTTPS forms of one repo give the same name. Credentials in the URL are dropped,
// never kept. Local paths and anything without an owner and a repo give "".
func NormaliseRemote(raw string) string {
	host, path := "", ""
	if m := scpLike.FindStringSubmatch(raw); m != nil {
		host, path = m[1], m[2]
	} else {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || defaultPorts[u.Scheme] == "" {
			return ""
		}
		host, path = u.Hostname(), u.Path
		if p := u.Port(); p != "" && p != defaultPorts[u.Scheme] {
			host += ":" + p
		}
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	if strings.Count(path, "/") < 1 || strings.Contains(path, "//") {
		return ""
	}
	return strings.ToLower(host) + "/" + path
}
