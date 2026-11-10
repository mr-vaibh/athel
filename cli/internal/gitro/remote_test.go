package gitro

import "testing"

func TestNormaliseRemote(t *testing.T) {
	tests := map[string]string{
		"git@github.com:acme/users.git":                               "github.com/acme/users",
		"git@GitHub.com:acme/Users":                                   "github.com/acme/Users",
		"ssh://git@github.com/acme/users.git":                         "github.com/acme/users",
		"ssh://git@github.com:22/acme/users":                          "github.com/acme/users",
		"ssh://git@git.acme.dev:2222/team/users.git":                  "git.acme.dev:2222/team/users",
		"https://github.com/acme/users.git":                           "github.com/acme/users",
		"https://github.com/acme/users/":                              "github.com/acme/users",
		"https://x-access-token:ghs_secret@github.com/acme/users.git": "github.com/acme/users",
		"http://gitlab.acme.dev/group/sub/users":                      "gitlab.acme.dev/group/sub/users",
		"git://github.com/acme/users":                                 "github.com/acme/users",
		"https://github.com:443/acme/users":                           "github.com/acme/users",
		"/srv/git/users.git":                                          "",
		"file:///srv/git/users.git":                                   "",
		"https://github.com/acme":                                     "",
		"git@github.com:":                                             "",
		"":                                                            "",
		"not a url":                                                   "",
	}
	for raw, want := range tests {
		if got := NormaliseRemote(raw); got != want {
			t.Errorf("NormaliseRemote(%q) = %q, want %q", raw, got, want)
		}
	}
}
