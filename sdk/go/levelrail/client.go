// Package levelrail is a small client for the Levelrail control plane API.
// The operation table is generated from docs/public/openapi.json; the methods
// here cover a core set, and Invoke reaches any other operation by id.
package levelrail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Operation describes one OpenAPI operation.
type Operation struct {
	Method  string
	Path    string
	HasBody bool
	Stream  bool
}

// Object is a decoded JSON object. Most responses are not typed in the spec.
type Object = map[string]any

// Client talks to one control plane with a bearer API token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	timeout time.Duration
	frame   int
	err     error
}

// DefaultTimeout bounds one non-streaming call; DefaultMaxFrameBytes bounds
// one SSE frame.
const (
	DefaultTimeout       = 30 * time.Second
	DefaultMaxFrameBytes = 4 << 20
)

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient replaces the default http.Client, for timeouts or transports.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the per-call timeout of the default client (streams exempt).
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithMaxFrameBytes caps the bytes of one SSE frame before ReadSSE errors.
func WithMaxFrameBytes(n int) Option { return func(c *Client) { c.frame = n } }

// New returns a client for baseURL (for example https://control.example.com).
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, timeout: DefaultTimeout, frame: DefaultMaxFrameBytes}
	c.http = &http.Client{}
	for _, o := range opts {
		o(c)
	}
	hc := *c.http
	if hc.Timeout == 0 {
		hc.Timeout = c.timeout
	}
	if hc.CheckRedirect == nil {
		hc.CheckRedirect = c.checkRedirect
	}
	c.http = &hc
	if u, err := url.Parse(c.baseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		c.err = fmt.Errorf("levelrail: base URL must be an absolute http or https URL")
	}
	return c
}

// checkRedirect refuses a TLS downgrade, and a host change while a token is set.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("levelrail: too many redirects")
	}
	prev := via[len(via)-1].URL
	if prev.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("levelrail: refusing redirect from https to http")
	}
	if c.token != "" && req.URL.Host != prev.Host {
		return errors.New("levelrail: refusing redirect to another host")
	}
	return nil
}

// APIError is a non-2xx response. The message never echoes request bodies.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("levelrail: HTTP %d: %s", e.Status, e.Message) }

type idemKey struct{}

// WithIdempotencyKey makes a create or deploy call safe to retry.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, idemKey{}, key)
}

// buildPath fills {placeholders} in order with escaped params.
func buildPath(tpl string, params []string) (string, error) {
	var b strings.Builder
	i := 0
	for rest := tpl; ; {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			b.WriteString(rest)
			break
		}
		cl := strings.IndexByte(rest[open:], '}')
		if cl < 0 || i >= len(params) {
			return "", fmt.Errorf("levelrail: path %q needs more parameters", tpl)
		}
		b.WriteString(rest[:open])
		b.WriteString(url.PathEscape(params[i]))
		i++
		rest = rest[open+cl+1:]
	}
	if i != len(params) {
		return "", fmt.Errorf("levelrail: path %q takes %d parameters, got %d", tpl, i, len(params))
	}
	return b.String(), nil
}

// request builds and sends one call. The caller closes the response body.
func (c *Client) request(ctx context.Context, opID string, params []string, q url.Values, body any, accept string) (*http.Response, error) {
	if c.err != nil {
		return nil, c.err
	}
	op, ok := Operations[opID]
	if !ok {
		return nil, fmt.Errorf("levelrail: unknown operation %q", opID)
	}
	p, err := buildPath(op.Path, params)
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("levelrail: encode body: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	u := c.baseURL + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, op.Method, u, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if k, ok := ctx.Value(idemKey{}).(string); ok && k != "" {
		req.Header.Set("Idempotency-Key", k)
	}
	hc := c.http
	if op.Stream {
		// A client timeout would cut a long-lived event stream.
		cp := *hc
		cp.Timeout = 0
		hc = &cp
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer func() { _ = resp.Body.Close() }()
		return nil, readAPIError(resp)
	}
	return resp, nil
}

func readAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var env struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &env) == nil && env.Error != "" {
		msg = env.Error
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}

// Invoke calls any operation by id and decodes the JSON response into out
// (nil to discard). It returns the response headers.
func (c *Client) Invoke(ctx context.Context, opID string, params []string, q url.Values, body, out any) (http.Header, error) {
	resp, err := c.request(ctx, opID, params, q, body, "application/json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return resp.Header, fmt.Errorf("levelrail: decode response: %w", err)
		}
	}
	return resp.Header, nil
}
