package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/ocisign"
	"github.com/glincker/levelrail/kit/httpx"
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
	// signer, when set, replaces bearer auth with a per-request OCI
	// signature, and error bodies are dropped so OCIDs never leak.
	signer *ocisign.Signer
	http   *http.Client
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

// newHTTPClientWithSigner returns an httpClient that signs every request
// (and every retry, so the Date header stays fresh) instead of sending a
// bearer token.
func newHTTPClientWithSigner(base string, signer ocisign.Signer) *httpClient {
	return &httpClient{
		base:   base,
		signer: &signer,
		http:   &http.Client{Timeout: requestTimeout},
		sleep:  contextSleep,
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
	// ociCode and ociMsg are kept off Error() so callers can branch on a
	// capacity error without the text ever reaching a log or API response.
	ociCode string
	ociMsg  string
}

func (e *ProviderError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("provision: provider returned %d", e.Status)
	}
	return fmt.Sprintf("provision: provider returned %d: %s", e.Status, e.Body)
}

// do sends method+path (relative to c.base) with body JSON-encoded (nil for
// none), decoding a 2xx response into out (nil to discard it). Retries on
// 429 up to maxRetries times, honoring a Retry-After header in seconds when
// the provider sends one.
func (c *httpClient) do(ctx context.Context, method, path string, body, out any) error {
	_, err := c.doHeaders(ctx, method, path, nil, body, out)
	return err
}

// doHeaders is do with extra request headers and the final response headers
// returned, so OCI callers can send a retry token and follow opc-next-page.
func (c *httpClient) doHeaders(ctx context.Context, method, path string, extra map[string]string, body, out any) (http.Header, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, fmt.Errorf("provision: encode request body: %w", err)
		}
	}

	bearer := c.token
	if c.tokenFunc != nil && c.signer == nil {
		t, err := c.tokenFunc(ctx)
		if err != nil {
			return nil, fmt.Errorf("provision: obtain access token: %w", err)
		}
		bearer = t
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("provision: build request: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		if c.signer != nil {
			if err := c.signer.Sign(req, payload); err != nil {
				return nil, fmt.Errorf("provision: %w", err)
			}
		} else {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("provision: request %s %s: %w", method, path, err)
		}
		respBody, readErr := httpx.ReadBody(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("provision: read response from %s %s: %w", method, path, readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			lastErr = c.providerError(resp.StatusCode, respBody)
			if sleepErr := c.sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return nil, fmt.Errorf("provision: %s %s: %w", method, path, sleepErr)
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, c.providerError(resp.StatusCode, respBody)
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return nil, fmt.Errorf("provision: decode response from %s %s: %w", method, path, err)
			}
		}
		return resp.Header, nil
	}
	return nil, fmt.Errorf("provision: %s %s: rate limited after %d retries: %w", method, path, maxRetries, lastErr)
}

// providerError builds the error for a non-2xx response. In signer mode the
// body is never kept, only the status plus the parsed code and message.
func (c *httpClient) providerError(status int, body []byte) *ProviderError {
	if c.signer == nil {
		return &ProviderError{Status: status, Body: string(body)}
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &e)
	return &ProviderError{Status: status, ociCode: e.Code, ociMsg: e.Message}
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
