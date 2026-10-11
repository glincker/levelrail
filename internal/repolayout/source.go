package repolayout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxContentReads bounds how many file bodies one detection fetches.
const maxContentReads = 16

// Source lists a repository's files at one ref and reads single files,
// without cloning the repository to disk.
type Source interface {
	// Paths returns every blob path. truncated is true when the provider
	// capped the listing.
	Paths(ctx context.Context) (paths []string, truncated bool, err error)
	Read(ctx context.Context, p string, limit int) ([]byte, error)
}

// Detect lists src and returns the layout analysis.
func Detect(ctx context.Context, src Source) (Result, error) {
	paths, truncated, err := src.Paths(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("repolayout: list files: %w", err)
	}
	reads := 0
	cache := map[string][]byte{}
	read := func(p string, limit int) ([]byte, error) {
		if b, ok := cache[p]; ok {
			return b, nil
		}
		if reads >= maxContentReads {
			return nil, errors.New("read budget exhausted")
		}
		reads++
		b, err := src.Read(ctx, p, limit)
		if err != nil {
			return nil, err
		}
		cache[p] = b
		return b, nil
	}
	res := Analyze(paths, read)
	res.Truncated = res.Truncated || truncated
	return res, nil
}

// Hosts names self-hosted instances so their API base can be inferred from a
// repository URL. The public hosts are always recognised.
type Hosts struct {
	GitHub string
	GitLab string
	Gitea  string
}

// ErrUnsupportedHost means no provider API matches the repository URL, and
// the caller should fall back to a shallow in-memory clone.
var ErrUnsupportedHost = errors.New("repolayout: no provider API for this host")

// NewAPISource picks a provider API source for repoURL. token may be empty
// for a public repository.
func NewAPISource(client *http.Client, repoURL, branch, token string, hosts Hosts) (Source, error) {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("repolayout: parse repository url: %w", ErrUnsupportedHost)
	}
	full := strings.Trim(strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git"), "/")
	if strings.Count(full, "/") < 1 {
		return nil, ErrUnsupportedHost
	}
	host := strings.ToLower(u.Host)
	base := u.Scheme + "://" + u.Host
	switch {
	case host == "github.com" || host == "www.github.com":
		return &githubSource{c: client, api: "https://api.github.com", repo: full, ref: branch, token: token}, nil
	case hosts.GitHub != "" && host == strings.ToLower(hosts.GitHub):
		return &githubSource{c: client, api: base + "/api/v3", repo: full, ref: branch, token: token}, nil
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab.") || (hosts.GitLab != "" && host == strings.ToLower(hosts.GitLab)):
		return &gitlabSource{c: client, api: base + "/api/v4", repo: full, ref: branch, token: token}, nil
	case host == "codeberg.org" || strings.HasPrefix(host, "gitea.") || (hosts.Gitea != "" && host == strings.ToLower(hosts.Gitea)):
		return &giteaSource{c: client, api: base + "/api/v1", repo: full, ref: branch, token: token}, nil
	}
	return nil, ErrUnsupportedHost
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func getJSON(ctx context.Context, c *http.Client, rawURL string, hdr map[string]string, out any) error {
	body, err := doGet(ctx, c, rawURL, hdr, 16<<20)
	if err != nil {
		return err
	}
	return decodeJSON(body, out)
}

func doGet(ctx context.Context, c *http.Client, rawURL string, hdr map[string]string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req) //nolint:gosec // host comes from the connected repository URL after the SSRF-guarded client check
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, &StatusError{Status: resp.StatusCode, Body: strings.TrimSpace(string(snippet))}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return b, nil
}

// StatusError is a non-2xx answer from a provider API.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("provider api returned %d: %s", e.Status, e.Body)
}
