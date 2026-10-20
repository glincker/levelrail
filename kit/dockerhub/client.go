// Package dockerhub is a minimal client for Docker Hub's public, unauthenticated
// API: repository search and tag listing.
package dockerhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	requestTimeout = 10 * time.Second
	defaultBaseURL = "https://hub.docker.com"
	// officialNamespace is the namespace Docker Hub's search API omits
	// from repo_name for official images (e.g. "postgres" rather than
	// "library/postgres"), but which the tags endpoint still requires
	// as an explicit path segment.
	officialNamespace = "library"
)

// ErrNotFound reports that Docker Hub has no such repository, the Hub
// API's own signal (a 404) for a namespace/name nothing was ever
// published under, distinct from an unreachable host or a rate limit.
var ErrNotFound = errors.New("dockerhub: repository not found")

// Repository is one GET /v2/search/repositories/ result: the fields the
// picker's search results list needs (name, description, popularity,
// official/automated flags), not the full upstream payload.
type Repository struct {
	Name             string `json:"repo_name"`
	ShortDescription string `json:"short_description"`
	StarCount        int    `json:"star_count"`
	IsOfficial       bool   `json:"is_official"`
	IsAutomated      bool   `json:"is_automated"`
}

// Tag is one GET /v2/repositories/{namespace}/{repo}/tags result: just
// the tag name, the same "name only" shape registrycatalog.ListTags
// returns for the built-in registry.
type Tag struct {
	Name string `json:"name"`
}

type searchResponse struct {
	Results []Repository `json:"results"`
}

type tagsResponse struct {
	Results []Tag `json:"results"`
}

// Client queries Docker Hub's public Hub API over plain HTTPS: no
// authentication, so no credentials to manage or leak.
type Client struct {
	HTTP *http.Client
	// BaseURL overrides defaultBaseURL, for tests only.
	BaseURL string
}

// NewClient returns a Client with a bounded per-request timeout, the
// same shape registrycatalog.NewClient establishes for its own single
// small JSON requests.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: requestTimeout}}
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

// SearchRepositories queries GET /v2/search/repositories/?query=...,
// returning Docker Hub's own relevance-ranked order unchanged.
func (c *Client) SearchRepositories(ctx context.Context, query string, pageSize int) ([]Repository, error) {
	target := fmt.Sprintf("%s/v2/search/repositories/?query=%s&page_size=%d",
		c.baseURL(), url.QueryEscape(query), pageSize)

	var out searchResponse
	if err := c.get(ctx, target, &out); err != nil {
		return nil, fmt.Errorf("dockerhub: search repositories %q: %w", query, err)
	}
	return out.Results, nil
}

// ListTags queries GET /v2/repositories/{namespace}/{repo}/tags. An
// empty namespace defaults to "library", the namespace Docker Hub's own
// search results omit for official images.
func (c *Client) ListTags(ctx context.Context, namespace, repository string, pageSize int) ([]Tag, error) {
	if namespace == "" {
		namespace = officialNamespace
	}
	target := fmt.Sprintf("%s/v2/repositories/%s/%s/tags?page_size=%d",
		c.baseURL(), url.PathEscape(namespace), url.PathEscape(repository), pageSize)

	var out tagsResponse
	if err := c.get(ctx, target, &out); err != nil {
		return nil, fmt.Errorf("dockerhub: list tags for %s/%s: %w", namespace, repository, err)
	}
	return out.Results, nil
}

func (c *Client) get(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, target)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response from %s: %w", target, err)
	}
	return nil
}
