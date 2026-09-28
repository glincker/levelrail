package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	maxRetries     = 3
	retryBaseDelay = 500 * time.Millisecond
)

// httpClient is the small REST helper hetzner.go and digitalocean.go both
// build on: bearer auth, JSON in and out, and a bounded retry on 429 so a
// burst of provisioning calls doesn't fail outright on a provider's rate
// limit.
type httpClient struct {
	base  string
	token string
	http  *http.Client
	// sleep is a seam for tests: real callers get time.Sleep, tests can
	// skip the wait entirely.
	sleep func(time.Duration)
}

func newHTTPClient(base, token string) *httpClient {
	return &httpClient{
		base:  base,
		token: token,
		http:  &http.Client{Timeout: requestTimeout},
		sleep: time.Sleep,
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

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("provision: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
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
			c.sleep(retryDelay(resp.Header.Get("Retry-After"), attempt))
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
// one, otherwise backs off by attempt number.
func retryDelay(retryAfter string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(retryAfter); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return retryBaseDelay * time.Duration(attempt+1)
}
