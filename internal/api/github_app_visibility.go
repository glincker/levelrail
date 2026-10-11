package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	githubDotComInstance = "https://github.com"
	githubDotComAPI      = "https://api.github.com"
	appVisibilityTimeout = 4 * time.Second
	appVisibilityTTL     = 5 * time.Minute
)

type appVisibilityCache struct {
	mu      sync.Mutex
	slug    string
	at      time.Time
	public  *bool
	fetched bool
}

// githubAppAPIBase is the REST root for an instance: api.github.com for
// github.com, /api/v3 under a GitHub Enterprise Server host.
func githubAppAPIBase(instanceURL string) string {
	base := strings.TrimSuffix(instanceURL, "/")
	if base == "" || base == githubDotComInstance {
		return githubDotComAPI
	}
	return base + "/api/v3"
}

// probeGitHubAppPublic asks GitHub whether anyone can install the App.
// GET /apps/{slug} answers without credentials only for a public App, so a
// 404 means private. Anything else is unknown and returns nil.
func probeGitHubAppPublic(ctx context.Context, apiBase, slug string) *bool {
	ctx, cancel := context.WithTimeout(ctx, appVisibilityTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/apps/"+url.PathEscape(slug), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: appVisibilityTimeout}).Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var public bool
	switch resp.StatusCode {
	case http.StatusOK:
		public = true
	case http.StatusNotFound:
		public = false
	default:
		return nil
	}
	return &public
}

func (rt *Router) githubAppPublic(ctx context.Context, instanceURL, slug string) *bool {
	c := &rt.appVisibility
	c.mu.Lock()
	if c.fetched && c.slug == slug && time.Since(c.at) < appVisibilityTTL {
		out := c.public
		c.mu.Unlock()
		return out
	}
	c.mu.Unlock()

	probe := rt.githubAppPublicProbe
	if probe == nil {
		probe = probeGitHubAppPublic
	}
	out := probe(ctx, githubAppAPIBase(instanceURL), slug)

	c.mu.Lock()
	if out != nil {
		c.slug, c.at, c.public, c.fetched = slug, time.Now(), out, true
	}
	c.mu.Unlock()
	return out
}

// githubAppMakePublicURL is the App's Advanced settings page, where the
// owner switches it to "Any account". The first installation is the
// owner's account in the normal flow, which says whether it is an org.
func githubAppMakePublicURL(instanceURL, slug, ownerType, ownerLogin string) string {
	base := strings.TrimSuffix(instanceURL, "/")
	if strings.EqualFold(ownerType, "Organization") && ownerLogin != "" {
		return base + "/organizations/" + url.PathEscape(ownerLogin) + "/settings/apps/" + url.PathEscape(slug) + "/advanced"
	}
	return base + "/settings/apps/" + url.PathEscape(slug) + "/advanced"
}
