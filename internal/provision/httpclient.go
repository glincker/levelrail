package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	maxRetries     = 3
	retryBaseDelay = 500 * time.Millisecond
	// maxRetryDelay bounds a provider-supplied Retry-After: an unbounded
	// numeric value would otherwise tie up a catalog or creation handler
	// for however long the provider asks, even past its own caller having
	// given up.
	maxRetryDelay = 30 * time.Second
)

// httpClient is the small REST helper every provider in this package
// builds on: bearer auth, JSON in and out, and a bounded retry on 429 so a
// burst of provisioning calls doesn't fail outright on a provider's rate
// limit.
type httpClient struct {
	base  string
	token string
	// tokenFunc, when set, is called before every request to obtain the
	// bearer token instead of the static token field: azure.go and gcp.go
	// use it for an OAuth2 access token that expires and must be
	// refreshed, unlike hetzner.go/digitalocean.go's long-lived static
	// token.
	tokenFunc func(context.Context) (string, error)
	http      *http.Client
	// sleep is a seam for tests: real callers get contextSleep, tests can
	// skip the wait entirely. Context-aware so a caller's cancellation
	// (or its own deadline) ends a retry wait immediately rather than
	// riding out a provider's own Retry-After.
	sleep func(context.Context, time.Duration) error
}

func newHTTPClient(base, token string) *httpClient {
	return &httpClient{
		base:  base,
		token: token,
		http:  &http.Client{Timeout: requestTimeout},
		sleep: contextSleep,
	}
}

// newHTTPClientWithTokenFunc returns an httpClient authenticating every
// request with a bearer token tokenFunc resolves fresh each time, for a
// provider whose credential is an OAuth2 token source rather than a
// static API token.
func newHTTPClientWithTokenFunc(base string, tokenFunc func(context.Context) (string, error)) *httpClient {
	return &httpClient{
		base:      base,
		tokenFunc: tokenFunc,
		http:      &http.Client{Timeout: requestTimeout},
		sleep:     contextSleep,
	}
}

// relativePath strips c.base from an absolute URL a paginated response
// handed back (DigitalOcean's own links.pages.next), so do can still be
// called the normal way instead of needing a second, absolute-URL entry
// point. Returns fullURL unchanged if it doesn't start with c.base.
func (c *httpClient) relativePath(fullURL string) string {
	if rest, ok := strings.CutPrefix(fullURL, c.base); ok {
		return rest
	}
	return fullURL
}

func contextSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ProviderError is a non-2xx response from a provider's API, carrying the
// HTTP status so callers (and tests) can distinguish "not found" from a
// generic failure without parsing each provider's own error body shape.
type ProviderError struct {
	Status int
	Body   string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provision: provider returned %d: %s", e.Status, e.Body)
}

// do sends method+path (relative to c.base) with body JSON-encoded (nil for
// none), decoding a 2xx response into out (nil to discard it). Retries on
// 429 up to maxRetries times, honoring a Retry-After header in seconds when
// the provider sends one.
func (c *httpClient) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("provision: encode request body: %w", err)
		}
	}

	bearer := c.token
	if c.tokenFunc != nil {
		t, err := c.tokenFunc(ctx)
		if err != nil {
			return fmt.Errorf("provision: obtain access token: %w", err)
		}
		bearer = t
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("provision: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("provision: request %s %s: %w", method, path, err)
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("provision: read response from %s %s: %w", method, path, readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			lastErr = &ProviderError{Status: resp.StatusCode, Body: string(respBody)}
			if sleepErr := c.sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return fmt.Errorf("provision: %s %s: %w", method, path, sleepErr)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &ProviderError{Status: resp.StatusCode, Body: string(respBody)}
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("provision: decode response from %s %s: %w", method, path, err)
			}
		}
		return nil
	}
	return fmt.Errorf("provision: %s %s: rate limited after %d retries: %w", method, path, maxRetries, lastErr)
}

// retryDelay honors a numeric Retry-After (seconds) when the provider sends
// one, otherwise backs off by attempt number. Capped at maxRetryDelay
// either way.
func retryDelay(retryAfter string, attempt int) time.Duration {
	d := retryBaseDelay * time.Duration(attempt+1)
	if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
		d = time.Duration(secs) * time.Second
	}
	if d > maxRetryDelay {
		d = maxRetryDelay
	}
	return d
}
