// Package registrycatalog implements a minimal client for the Docker
// Registry HTTP API v2's catalog endpoints (_catalog, <name>/tags/list),
// used to browse repositories and tags in Levelrail's own built-in
// registry (internal/reconcile/registry) from the app-creation UI.
// Deliberately scoped to that one registry: see internal/api's
// RegistryCatalogClient interface doc comment for why an arbitrary
// external registry's catalog isn't queried the same way.
package registrycatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const requestTimeout = 10 * time.Second

// ErrNotFound reports that the registry has no such repository, the v2
// API's own signal (a 404 from tags/list) for a name nothing has ever
// been pushed to, distinct from an unreachable registry or a rejected
// credential.
var ErrNotFound = errors.New("registrycatalog: repository not found")

// Client queries a Docker Registry HTTP API v2 server's catalog
// endpoints over Basic Auth.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a bounded per-request timeout: every
// call here is a single small JSON request, not a long-lived stream, the
// same shape githubapp.NewClient establishes for its own client.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: requestTimeout}}
}

type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

type tagsListResponse struct {
	Tags []string `json:"tags"`
}

// ListRepositories queries GET {baseURL}/v2/_catalog, returning every
// repository name the registry knows about, sorted for a stable UI
// order (the wire response's own ordering is unspecified).
func (c *Client) ListRepositories(ctx context.Context, baseURL, username, password string) ([]string, error) {
	var out catalogResponse
	if err := c.get(ctx, baseURL+"/v2/_catalog", username, password, &out); err != nil {
		return nil, fmt.Errorf("registrycatalog: list repositories: %w", err)
	}
	sort.Strings(out.Repositories)
	return out.Repositories, nil
}

// ListTags queries GET {baseURL}/v2/{repository}/tags/list. repository
// may itself contain slashes (a namespaced name like "org/app"): it is
// path-escaped per segment, not as a whole, so those slashes still
// separate real path segments on the wire.
func (c *Client) ListTags(ctx context.Context, baseURL, username, password, repository string) ([]string, error) {
	var out tagsListResponse
	target := baseURL + "/v2/" + escapeRepositoryPath(repository) + "/tags/list"
	if err := c.get(ctx, target, username, password, &out); err != nil {
		return nil, fmt.Errorf("registrycatalog: list tags for %q: %w", repository, err)
	}
	sort.Strings(out.Tags)
	return out.Tags, nil
}

// escapeRepositoryPath path-escapes each "/"-separated segment of a
// repository name independently, so a namespaced name like "org/app"
// keeps its real path structure on the wire instead of becoming one
// escaped opaque segment.
func escapeRepositoryPath(repository string) string {
	segments := strings.Split(repository, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func (c *Client) get(ctx context.Context, target, username, password string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(username, password)
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
