package platformimport

import (
	"strings"
	"testing"
)

func TestGitURLFromRepo(t *testing.T) {
	cases := map[string]string{
		"acme/web":                                "https://github.com/acme/web",
		"github.com/acme/blog":                    "https://github.com/acme/blog",
		"git@github.com:acme/api.git":             "https://github.com/acme/api",
		"ssh://git@github.com/acme/api.git":       "https://github.com/acme/api",
		"ssh://git@git.example.com:2222/team/app": "https://git.example.com/team/app",
		"git@gitlab.com:group/sub/proj.git":       "https://gitlab.com/group/sub/proj",
		"https://gitlab.example.org/acme/api.git": "https://gitlab.example.org/acme/api.git",
		"file:///srv/repo.git":                    "file:///srv/repo.git",
		"  ":                                      "",
	}
	for in, want := range cases {
		if got := gitURLFromRepo(in); got != want {
			t.Errorf("gitURLFromRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSetGitURLNotesSSHMapping(t *testing.T) {
	var a App
	a.setGitURL("git@github.com:acme/api.git")
	if a.GitURL != "https://github.com/acme/api" || len(a.Notes) != 1 || !strings.Contains(a.Notes[0].Reason, "over ssh") {
		t.Fatalf("ssh source: %q %+v", a.GitURL, a.Notes)
	}
	var b App
	b.setGitURL("https://github.com/acme/api")
	if b.GitURL != "https://github.com/acme/api" || len(b.Notes) != 0 {
		t.Fatalf("https source: %q %+v", b.GitURL, b.Notes)
	}
}
