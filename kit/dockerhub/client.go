// Package dockerhub is a minimal Docker Hub client: public repository search
// and tag listing, plus an optional token (PAT or OAT) for private
// repositories, rate limit status and tag to digest resolution.
package dockerhub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
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
	maxBody           = 4 << 20
)

// ErrNotFound reports that Docker Hub has no such repository, the Hub
// API's own signal (a 404) for a namespace/name nothing was ever
// published under, distinct from an unreachable host or a rate limit.
var ErrNotFound = errors.New("dockerhub: repository not found")

// ErrUnauthorized reports that Docker Hub rejected the token (HTTP 401).
var ErrUnauthorized = errors.New("dockerhub: token rejected")

// ErrForbidden reports that the token is valid but lacks the scope or
// repository permission the call needs (HTTP 403).
var ErrForbidden = errors.New("dockerhub: token lacks the required scope")

// Repository is one GET /v2/search/repositories/ result: the fields the
// picker's search results list needs (name, description, popularity,
// official/automated flags), not the full upstream payload.
type Repository struct {
	Name             string `json:"repo_name"`
	ShortDescription string `json:"short_description"`
	StarCount        int    `json:"star_count"`
	IsOfficial       bool   `json:"is_official"`
	IsAutomated      bool   `json:"is_automated"`
	// IsPrivate is set only for repositories read with a token.
	IsPrivate bool `json:"is_private"`
}

// Tag is one image tag. Anonymous listings fill Name only; credentialed
// listings add digest, size, architectures and push time.
type Tag struct {
	Name          string   `json:"name"`
	Digest        string   `json:"digest,omitempty"`
	FullSize      int64    `json:"full_size"`
	LastUpdated   string   `json:"last_updated,omitempty"`
	LastPushed    string   `json:"last_pushed,omitempty"`
	Architectures []string `json:"architectures"`
	Images        []Image  `json:"images"`
}

// Image is one platform entry of a multi-architecture tag.
type Image struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant,omitempty"`
	Digest       string `json:"digest,omitempty"`
	Size         int64  `json:"size"`
}

type searchResponse struct {
	Results []Repository `json:"results"`
}

type tagsResponse struct {
	Count   int      `json:"count"`
	Next    *string  `json:"next"`
	Results []rawTag `json:"results"`
}

// Client queries Docker Hub's public Hub API over plain HTTPS: no
// authentication, so no credentials to manage or leak.
type Client struct {
	HTTP *http.Client
	// BaseURL, AuthURL and RegistryURL override the Hub API, token and
	// registry hosts, for tests only.
	BaseURL     string
	AuthURL     string
	RegistryURL string

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewClient returns a Client with a bounded per-request timeout, the
// same shape registrycatalog.NewClient establishes for its own single
// small JSON requests.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: requestTimeout, Transport: hostGuard{next: http.DefaultTransport}}}
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
	if err := c.doJSON(ctx, http.MethodGet, target, "", nil, &out); err != nil {
		return nil, fmt.Errorf("dockerhub: list tags for %s/%s: %w", namespace, repository, err)
	}
	return convertTags(out.Results), nil
}

func (c *Client) get(ctx context.Context, target string, out any) error {
	return c.doJSON(ctx, http.MethodGet, target, "", nil, out)
}

// doJSON sends one request, optionally with a bearer and a JSON body, and
// maps the status to the package errors.
func (c *Client) doJSON(ctx context.Context, method, target, bearer string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	_, err = c.send(req, out)
	return err
}

func (c *Client) send(req *http.Request, out any) (http.Header, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", req.Method, req.URL.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp); err != nil {
		return resp.Header, err
	}
	if out == nil {
		return resp.Header, nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return resp.Header, fmt.Errorf("decode response from %s: %w", req.URL.Host, err)
	}
	return resp.Header, nil
}

func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusForbidden:
		return ErrForbidden
	case resp.StatusCode == http.StatusTooManyRequests:
		return &RateLimitedError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, resp.Request.URL.Host)
}
