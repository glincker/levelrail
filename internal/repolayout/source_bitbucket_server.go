package repolayout

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DefaultBrancher is implemented by sources whose provider can name the
// repository's default branch, so callers need not guess main or master.
type DefaultBrancher interface {
	DefaultBranch(ctx context.Context) (string, error)
}

func isBitbucketServerHost(host string, hosts Hosts) bool {
	if hosts.BitbucketServer != "" && host == strings.ToLower(hosts.BitbucketServer) {
		return true
	}
	return strings.HasPrefix(host, "bitbucket.") && host != "bitbucket.org"
}

type bitbucketServerSource struct {
	c                    *http.Client
	api, ref, token      string
	projectKey, repoSlug string
}

// newBitbucketServerSource accepts a clone URL (/scm/PROJ/repo.git) or a web
// URL (/projects/PROJ/repos/repo/...), with an optional context path before either.
func newBitbucketServerSource(client *http.Client, u *url.URL, branch, token string) (Source, error) {
	path := strings.Trim(u.Path, "/")
	var prefix, project, slug string
	if i := strings.Index(path, "scm/"); i >= 0 && (i == 0 || path[i-1] == '/') {
		prefix = strings.Trim(path[:i], "/")
		project, slug, _ = strings.Cut(strings.TrimSuffix(path[i+len("scm/"):], ".git"), "/")
	} else if i := strings.Index(path, "projects/"); i >= 0 && (i == 0 || path[i-1] == '/') {
		parts := strings.Split(path[i+len("projects/"):], "/")
		if len(parts) >= 3 && parts[1] == "repos" {
			prefix, project, slug = strings.Trim(path[:i], "/"), parts[0], parts[2]
		}
	}
	if project == "" || slug == "" || strings.Contains(slug, "/") {
		return nil, ErrUnsupportedHost
	}
	base := u.Scheme + "://" + u.Host
	if prefix != "" {
		base += "/" + prefix
	}
	return &bitbucketServerSource{c: client, api: base + "/rest/api/1.0", ref: branch, token: token, projectKey: project, repoSlug: slug}, nil
}

func (s *bitbucketServerSource) headers() map[string]string {
	h := map[string]string{}
	if s.token != "" {
		h["Authorization"] = "Bearer " + s.token
	}
	return h
}

func (s *bitbucketServerSource) repoURL() string {
	return s.api + "/projects/" + url.PathEscape(s.projectKey) + "/repos/" + url.PathEscape(s.repoSlug)
}

// DefaultBranch asks the server which branch is the default.
func (s *bitbucketServerSource) DefaultBranch(ctx context.Context) (string, error) {
	var out struct {
		DisplayID string `json:"displayId"`
	}
	if err := getJSON(ctx, s.c, s.repoURL()+"/default-branch", s.headers(), &out); err != nil {
		return "", err
	}
	return out.DisplayID, nil
}

func (s *bitbucketServerSource) Paths(ctx context.Context) ([]string, bool, error) {
	var out []string
	start := 0
	for page := 0; page < maxListPages; page++ {
		var resp struct {
			Values        []string `json:"values"`
			IsLastPage    bool     `json:"isLastPage"`
			NextPageStart int      `json:"nextPageStart"`
		}
		u := fmt.Sprintf("%s/files?limit=1000&start=%d&at=%s", s.repoURL(), start, url.QueryEscape(s.ref))
		if err := getJSON(ctx, s.c, u, s.headers(), &resp); err != nil {
			return nil, false, err
		}
		out = append(out, resp.Values...)
		if resp.IsLastPage {
			return out, false, nil
		}
		start = resp.NextPageStart
	}
	return out, true, nil
}

func (s *bitbucketServerSource) Read(ctx context.Context, p string, limit int) ([]byte, error) {
	u := s.repoURL() + "/raw/" + escapePath(p) + "?at=" + url.QueryEscape(s.ref)
	return readCapped(ctx, s.c, u, s.headers(), limit)
}
