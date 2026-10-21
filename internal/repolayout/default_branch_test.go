package repolayout

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDefaultBranchPerProvider(t *testing.T) {
	cases := []struct {
		name, path, body string
		build            func(c *http.Client, base string) DefaultBrancher
	}{
		{"github", "/repos/o/r", `{"default_branch":"trunk"}`, func(c *http.Client, b string) DefaultBrancher {
			return &githubSource{c: c, api: b, repo: "o/r"}
		}},
		{"gitlab", "/projects/g%2Fr", `{"default_branch":"trunk"}`, func(c *http.Client, b string) DefaultBrancher {
			return &gitlabSource{c: c, api: b, repo: "g/r"}
		}},
		{"gitea", "/repos/o/r", `{"default_branch":"trunk"}`, func(c *http.Client, b string) DefaultBrancher {
			return &giteaSource{c: c, api: b, repo: "o/r"}
		}},
		{"bitbucket server", "/rest/api/1.0/projects/P/repos/r/default-branch", `{"displayId":"trunk"}`, func(c *http.Client, b string) DefaultBrancher {
			return &bitbucketServerSource{c: c, api: b + "/rest/api/1.0", projectKey: "P", repoSlug: "r"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tc.path {
					http.NotFound(w, r)
					return
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			got, err := tc.build(srv.Client(), srv.URL).DefaultBranch(context.Background())
			if err != nil || got != "trunk" {
				t.Fatalf("got %q, err %v", got, err)
			}
		})
	}
}

func TestBitbucketServerSourceURLs(t *testing.T) {
	cases := []struct {
		raw, api, project, slug string
		ok                      bool
	}{
		{"https://bitbucket.corp/scm/proj/app.git", "https://bitbucket.corp/rest/api/1.0", "proj", "app", true},
		{"https://code.corp/bitbucket/scm/PROJ/app.git", "https://code.corp/bitbucket/rest/api/1.0", "PROJ", "app", true},
		{"https://bitbucket.corp/projects/PROJ/repos/app/browse", "https://bitbucket.corp/rest/api/1.0", "PROJ", "app", true},
		{"https://bitbucket.corp/team/app", "", "", "", false},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.raw)
		src, err := newBitbucketServerSource(http.DefaultClient, u, "main", "")
		if (err == nil) != tc.ok {
			t.Fatalf("%s: err = %v", tc.raw, err)
		}
		if !tc.ok {
			continue
		}
		s := src.(*bitbucketServerSource)
		if s.api != tc.api || s.projectKey != tc.project || s.repoSlug != tc.slug {
			t.Errorf("%s: got %+v", tc.raw, s)
		}
	}
}

func TestBitbucketServerPathsAndRead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/1.0/projects/P/repos/r/files":
			if r.URL.Query().Get("at") != "dev" {
				t.Errorf("at = %q", r.URL.Query().Get("at"))
			}
			_, _ = io.WriteString(w, `{"values":["Dockerfile","src/a.go"],"isLastPage":true}`)
		case "/rest/api/1.0/projects/P/repos/r/raw/Dockerfile":
			_, _ = io.WriteString(w, "FROM scratch")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := &bitbucketServerSource{c: srv.Client(), api: srv.URL + "/rest/api/1.0", ref: "dev", projectKey: "P", repoSlug: "r"}
	paths, trunc, err := s.Paths(context.Background())
	if err != nil || trunc || len(paths) != 2 {
		t.Fatalf("paths = %v, trunc = %v, err = %v", paths, trunc, err)
	}
	b, err := s.Read(context.Background(), "Dockerfile", 1024)
	if err != nil || string(b) != "FROM scratch" {
		t.Fatalf("read = %q, err = %v", b, err)
	}
}
