package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Retry defaults, overridable from the provider block.
const (
	defaultMaxRetries   = 4
	defaultRetryWaitMax = 30 * time.Second
	retryWaitBase       = 500 * time.Millisecond
)

// Client is a minimal JSON client for the control plane REST API.
type Client struct {
	base  string
	token string
	http  *http.Client

	maxRetries   int
	retryWaitMax time.Duration
	// sleep is a field so tests can run the backoff on a fake clock.
	sleep func(ctx context.Context, d time.Duration) error
}

// NewClient returns a client for the control plane at baseURL.
func NewClient(baseURL, token string) *Client {
	return &Client{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		http:  &http.Client{Timeout: 60 * time.Second},

		maxRetries:   defaultMaxRetries,
		retryWaitMax: defaultRetryWaitMax,
		sleep:        sleepCtx,
	}
}

// withRetry sets the retry budget; zero retries turns retrying off.
func (c *Client) withRetry(maxRetries int, waitMax time.Duration) *Client {
	c.maxRetries = max(maxRetries, 0)
	if waitMax > 0 {
		c.retryWaitMax = waitMax
	}
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryable reports whether a request may be sent again: reads and PUT/DELETE
// always, POST only with an Idempotency-Key so a replay cannot double-create.
func retryable(method string, hasKey bool) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete:
		return true
	case http.MethodPost:
		return hasKey
	}
	return false
}

// backoff is exponential with full jitter, raised to Retry-After when the
// server asks for more, and never above the configured cap.
func (c *Client) backoff(attempt int, retryAfter time.Duration) time.Duration {
	d := retryWaitBase << min(attempt, 10)
	d = time.Duration(rand.Int64N(int64(d)/2+1)) + d/2
	d = max(d, retryAfter)
	return min(d, c.retryWaitMax)
}

func parseRetryAfter(h string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		return max(time.Until(t), 0)
	}
	return 0
}

// APIError is a non-2xx answer from the control plane.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

func isNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

func esc(s string) string { return url.PathEscape(s) }

// do sends one request. body and out are JSON-encoded values of any shape
// (the API has no single response type), either may be nil.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	return c.doKeyed(ctx, method, path, "", body, out)
}

// doKeyed is do with an Idempotency-Key, which makes a POST safe to retry.
func (c *Client) doKeyed(ctx context.Context, method, path, idemKey string, body, out any) error {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	canRetry := retryable(method, idemKey != "")
	for attempt := 0; ; attempt++ {
		wait, err := c.once(ctx, method, path, idemKey, raw, out)
		if err == nil || !canRetry || attempt >= c.maxRetries || wait < 0 {
			return err
		}
		if serr := c.sleep(ctx, c.backoff(attempt, wait)); serr != nil {
			return err
		}
	}
}

// once sends a single attempt. The returned wait is -1 when the failure must
// not be retried, otherwise the server's Retry-After (0 when absent).
func (c *Client) once(ctx context.Context, method, path, idemKey string, raw []byte, out any) (time.Duration, error) {
	var rd io.Reader
	if raw != nil {
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return -1, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if raw != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		wait := time.Duration(0)
		if ctx.Err() != nil {
			wait = -1
		}
		return wait, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		ae := &APIError{Status: resp.StatusCode, Message: errorMessage(data, resp.Status)}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return parseRetryAfter(resp.Header.Get("Retry-After")), ae
		}
		return -1, ae
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return -1, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return -1, fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return -1, nil
}

func errorMessage(data []byte, fallback string) string {
	var e struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil {
		if e.Message != "" {
			return e.Message
		}
		switch v := e.Error.(type) {
		case string:
			return v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				return m
			}
		}
	}
	return fallback
}

type wireProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type wireEnvironment struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
}

type wireDatabase struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	ProjectID string `json:"project_id"`
	Pooler    bool   `json:"pooler_enabled"`
	PoolSize  int64  `json:"pooler_pool_size"`
	MaxConn   int64  `json:"pooler_max_client_conn"`
}

type wireApp struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Port        int64             `json:"port"`
	BindAddress string            `json:"bind_address"`
	Strategy    string            `json:"strategy"`
	Replicas    int64             `json:"replicas"`
	Domains     []string          `json:"domains"`
	Env         map[string]string `json:"env"`
	ProjectID   string            `json:"project_id"`
	Command     []string          `json:"command"`
	Labels      map[string]string `json:"labels"`
	Resources   struct {
		MemoryBytes int64 `json:"memory_bytes"`
		NanoCPUs    int64 `json:"nano_cpus"`
	} `json:"resources"`
	Health struct {
		Restart   string     `json:"restart"`
		Readiness *wireProbe `json:"readiness"`
		Liveness  *wireProbe `json:"liveness"`
	} `json:"health"`
	Volumes []struct {
		Name          string `json:"name"`
		ContainerPath string `json:"container_path"`
	} `json:"volumes"`
	Tags           []string `json:"tags"`
	PublishedPorts []struct {
		ContainerPort int64  `json:"container_port"`
		HostPort      int64  `json:"host_port"`
		Protocol      string `json:"protocol"`
		Bind          string `json:"bind"`
	} `json:"published_ports"`
}

type wireProbe struct {
	Path string   `json:"path"`
	Exec []string `json:"exec"`
	TCP  bool     `json:"tcp"`
}

type wireNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Status      string `json:"status"`
	Region      string `json:"region"`
	Schedulable bool   `json:"schedulable"`
}

// envPath maps an env var scope to its endpoint.
func envPath(scope, id string) (string, error) {
	switch scope {
	case "project":
		return "/api/v1/projects/" + esc(id) + "/env", nil
	case "environment":
		return "/api/v1/environments/" + esc(id) + "/env", nil
	}
	return "", fmt.Errorf("scope must be \"project\" or \"environment\", got %q", scope)
}

func (c *Client) getApp(ctx context.Context, name string) (wireApp, map[string]any, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(name), nil, &raw); err != nil {
		return wireApp{}, nil, err
	}
	var app wireApp
	buf, _ := json.Marshal(raw)
	if err := json.Unmarshal(buf, &app); err != nil {
		return wireApp{}, nil, fmt.Errorf("decode app %s: %w", name, err)
	}
	return app, raw, nil
}

// putApp saves a full app body, which the API replaces wholesale.
func (c *Client) putApp(ctx context.Context, name string, body map[string]any) error {
	return c.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(name), body, nil)
}

func (c *Client) listEnvironments(ctx context.Context, projectID string) ([]wireEnvironment, error) {
	var envs []wireEnvironment
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+esc(projectID)+"/environments", nil, &envs)
	return envs, err
}
