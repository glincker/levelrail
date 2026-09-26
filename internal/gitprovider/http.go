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
	"strings"
)

// IsRateLimited reports whether err is a provider API error caused by rate
// limiting, and the Retry-After value it carried, if any.
func IsRateLimited(err error) (retryAfter string, ok bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RateLimited {
		return apiErr.RetryAfter, true
	}
	return "", false
}

func rateLimited(resp *http.Response, body string) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(body), "rate limit")
}

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
	// RetryAfter is the response's Retry-After header, empty when absent.
	RetryAfter string
	// RateLimited is true for a 429, or a 403 the provider marks as rate
	// limited (an exhausted X-RateLimit-Remaining or a "rate limit" body).
	RateLimited bool
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
		return &APIError{
			Prefix: prefix, API: api, StatusCode: resp.StatusCode, Body: string(snippet),
			RetryAfter:  resp.Header.Get("Retry-After"),
			RateLimited: rateLimited(resp, string(snippet)),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s: decode response for %s: %w", prefix, label, err)
	}
	return nil
}
