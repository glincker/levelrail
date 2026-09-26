// Package gitprovider holds the small slice of REST-client plumbing that
// is genuinely identical across internal/githubapp, internal/gitlabapp,
// and internal/bitbucketapp: sending a built request and turning its
// response into either a decoded value or a uniform API error. Request
// construction (auth scheme, headers, pagination shape) stays in each
// provider's own package, since that is exactly where the three
// providers' real, non-coincidental differences live.
package gitprovider

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// MaxErrorBodySnippet caps how much of a non-2xx response body an
// APIError retains, so a misbehaving upstream can't inflate a log line
// unboundedly.
const MaxErrorBodySnippet = 512

// APIError is returned for any non-2xx response from a git provider's
// REST API. Prefix and API identify the caller (e.g. "gitlabapp",
// "gitlab") so Error() reproduces that client's own message shape.
type APIError struct {
	Prefix     string
	API        string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s api returned %d: %s", e.Prefix, e.API, e.StatusCode, e.Body)
}

// Execute sends req and, on a 2xx response, decodes its body into out
// (skipped when out is nil). prefix and api identify the caller for both
// wrapped errors and a returned *APIError's own message; label is the
// method/URL text those wrapped errors describe, since GitHub's own
// caller wraps a relative path while GitLab's and Bitbucket's wrap a full
// URL.
func Execute(client *http.Client, req *http.Request, prefix, api, label string, out any) error {
	resp, err := client.Do(req) //nolint:gosec // req's URL was built by the caller from its own operator-configured API host, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("%s: request %s: %w", prefix, label, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodySnippet))
		return &APIError{Prefix: prefix, API: api, StatusCode: resp.StatusCode, Body: string(snippet)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response for %s: %w", prefix, label, err)
	}
	return nil
}

// Comment is one pull request comment as the preview upsert sees it.
type Comment struct {
	ID   int64
	Body string
}

// MaxCommentPages bounds how many pages a comment lookup walks, so a PR
// with a huge thread cannot turn one status update into unbounded requests.
const MaxCommentPages = 10

// CommentPageSize is the page size every provider's comment listing asks for.
const CommentPageSize = 100

// IsNotFound reports whether err is a provider 404, meaning the resource
// (typically a comment an operator deleted) no longer exists.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
