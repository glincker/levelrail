package reporef

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in, scheme, host, port, owner, repo, clone string
		provider                                   Provider
		creds                                      bool
	}{
		{"https://github.com/acme/shop", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"https://github.com/acme/shop.git", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"https://github.com/acme/shop.git/", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"  https://GitHub.com/Acme/Shop  ", SchemeHTTPS, "github.com", "", "Acme", "Shop", "https://github.com/Acme/Shop", ProviderGitHub, false},
		{"https://x-access-token:ghs_abc@github.com/acme/shop.git", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, true},
		{"https://ghp_token@github.com/acme/shop", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, true},
		{"https://github.com/acme/shop?tab=readme#top", SchemeHTTPS, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"http://gitea.lan.example:3000/team/api.git", SchemeHTTP, "gitea.lan.example", "3000", "team", "api", "http://gitea.lan.example:3000/team/api", ProviderOther, false},
		{"https://git.example.com:8443/team/api", SchemeHTTPS, "git.example.com", "8443", "team", "api", "https://git.example.com:8443/team/api", ProviderOther, false},
		{"git@github.com:acme/shop.git", SchemeSCP, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"git@github.com:acme/shop", SchemeSCP, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"github.com:acme/shop.git", SchemeSCP, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"git@github.com:/acme/shop.git", SchemeSCP, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"org-123@github.com:acme/shop.git", SchemeSCP, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"git@gitlab.com:group/sub/proj.git", SchemeSCP, "gitlab.com", "", "group/sub", "proj", "https://gitlab.com/group/sub/proj", ProviderGitLab, false},
		{"git@bitbucket.org:team/repo.git", SchemeSCP, "bitbucket.org", "", "team", "repo", "https://bitbucket.org/team/repo", ProviderBitbucket, false},
		{"git@ssh.dev.azure.com:v3/org/project/repo", SchemeSCP, "dev.azure.com", "", "org/project/_git", "repo", "https://dev.azure.com/org/project/_git/repo", ProviderOther, false},
		{"ssh://git@github.com/acme/shop.git", SchemeSSH, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"ssh://git@ssh.github.com:443/acme/shop.git", SchemeSSH, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"ssh://git@altssh.gitlab.com:443/group/proj.git", SchemeSSH, "gitlab.com", "", "group", "proj", "https://gitlab.com/group/proj", ProviderGitLab, false},
		{"ssh://git@git.example.com:2222/org/app", SchemeSSH, "git.example.com", "", "org", "app", "https://git.example.com/org/app", ProviderOther, false},
		{"git+ssh://git@github.com/acme/shop.git", SchemeSSH, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"ssh+git://git@github.com/acme/shop", SchemeSSH, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"git://github.com/acme/shop", SchemeGit, "github.com", "", "acme", "shop", "https://github.com/acme/shop", ProviderGitHub, false},
		{"https://github.example.corp/acme/shop", SchemeHTTPS, "github.example.corp", "", "acme", "shop", "https://github.example.corp/acme/shop", ProviderGitHub, false},
		{"https://acme.ghe.com/team/shop", SchemeHTTPS, "acme.ghe.com", "", "team", "shop", "https://acme.ghe.com/team/shop", ProviderGitHub, false},
		{"https://gitlab.example.org/acme/api.git", SchemeHTTPS, "gitlab.example.org", "", "acme", "api", "https://gitlab.example.org/acme/api", ProviderGitLab, false},
		{"https://bitbucket.example.com/scm/proj/repo.git", SchemeHTTPS, "bitbucket.example.com", "", "scm/proj", "repo", "https://bitbucket.example.com/scm/proj/repo", ProviderBitbucket, false},
		{"https://codeberg.org/forgejo/forgejo", SchemeHTTPS, "codeberg.org", "", "forgejo", "forgejo", "https://codeberg.org/forgejo/forgejo", ProviderOther, false},
		{"https://git.example.com/solo", SchemeHTTPS, "git.example.com", "", "", "solo", "https://git.example.com/solo", ProviderOther, false},
	}
	for _, c := range cases {
		r, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) error %v", c.in, err)
			continue
		}
		if r.Scheme != c.scheme || r.Host != c.host || r.Port != c.port || r.Owner != c.owner || r.Repo != c.repo || r.Provider != c.provider || r.HadCredentials != c.creds {
			t.Errorf("Parse(%q) = %+v", c.in, r)
		}
		if got := r.CloneURL(); got != c.clone {
			t.Errorf("Parse(%q).CloneURL() = %q, want %q", c.in, got, c.clone)
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{
		"", "   ", "file:///srv/repo.git", "ext::sh -c touch% /tmp/x", "/srv/repo.git", "./repo", "../repo",
		`C:\code\repo`, "acme/shop", "myalias:acme/shop", "https://github.com", "https://github.com/", "https://github.com/a/../b",
		"https://github.com/a//b", "ftp://example.com/a/b", "git@github.com:", "https://exa mple.com/a/b",
	} {
		if r, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %+v, want error", in, r)
		} else if !errors.Is(err, ErrUnsupported) {
			t.Errorf("Parse(%q) error %v is not ErrUnsupported", in, err)
		}
	}
}

func TestParseWithHost(t *testing.T) {
	cases := map[string]string{
		"acme/web":                    "https://github.com/acme/web",
		"/acme/web.git/":              "https://github.com/acme/web",
		"github.com/acme/blog":        "https://github.com/acme/blog",
		"gitlab.example.org/g/s/proj": "https://gitlab.example.org/g/s/proj",
		"git@github.com:acme/api.git": "https://github.com/acme/api",
		"https://gitlab.com/acme/api": "https://gitlab.com/acme/api",
	}
	for in, want := range cases {
		r, err := ParseWithHost(in, "github.com")
		if err != nil || r.CloneURL() != want {
			t.Errorf("ParseWithHost(%q) = %q, %v, want %q", in, r.CloneURL(), err, want)
		}
	}
	if _, err := ParseWithHost("acme/web", ""); err == nil {
		t.Error("shorthand without a default host accepted")
	}
	if _, err := ParseWithHost("file:///x/y", "github.com"); err == nil {
		t.Error("file URL accepted")
	}
}

func TestIdentity(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Org/Web.git":          "org/web",
		"git@github.com:org/web.git":              "org/web",
		"ssh://git@github.com/org/web":            "org/web",
		"org/web":                                 "org/web",
		"https://gitlab.com/group/sub/proj.git/":  "group/sub/proj",
		"https://user:tok@github.com/org/web.git": "org/web",
		"":              "",
		"file:///srv/x": "",
	}
	for in, want := range cases {
		if got := Identity(in); got != want {
			t.Errorf("Identity(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"https://github.com/a/b.git", "git@gitlab.com:g/s/p.git", "ssh://git@h.example:22/o/r", "acme/web", "https://u:p@h.example/o/r?x#y"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		r, err := Parse(in)
		if err != nil {
			return
		}
		clone := r.CloneURL()
		if strings.Contains(clone, "@") || strings.ContainsAny(clone, "?# ") {
			t.Fatalf("Parse(%q).CloneURL() = %q carries userinfo, query or space", in, clone)
		}
		again, err := Parse(clone)
		if err != nil || again.CloneURL() != clone || again.Identity() != r.Identity() {
			t.Fatalf("Parse(%q) clone %q does not round trip: %+v %v", in, clone, again, err)
		}
	})
}
