package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// serviceResources and serviceHealth/serviceProbe mirror
// internal/api's appResource wire shape (internal/api/apps.go), which is
// itself store.DesiredService's own JSON encoding: bytes and
// nanoseconds, not app.yaml's human-friendly "512Mi"/"5s" strings.
// Redeclared here, not imported from internal/store or internal/api, so
// this binary depends only on the documented wire contract, never on the
// control plane's internal Go types: the CLI and a future MCP server are
// exactly the external callers that contract exists for.
type serviceResources struct {
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
}

type serviceProbe struct {
	Path     string `json:"path"`
	Interval int64  `json:"interval,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

type serviceHealth struct {
	Readiness *serviceProbe `json:"readiness,omitempty"`
	Liveness  *serviceProbe `json:"liveness,omitempty"`
}

// appResource mirrors internal/api's appResource (apps.go). Field order
// and JSON tags match exactly, so a response decodes cleanly and a
// request encodes into exactly what the server expects.
type appResource struct {
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Port      int               `json:"port"`
	Domains   []string          `json:"domains,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources *serviceResources `json:"resources,omitempty"`
	Health    *serviceHealth    `json:"health,omitempty"`
	NodeID    string            `json:"node_id,omitempty"`
}

// buildTriggerRequest mirrors internal/api's triggerBuildRequest
// (internal/api/builds.go). Ref is required server-side (handleTriggerBuild
// rejects an empty one with 400), so every caller of TriggerBuild must
// resolve a real ref before sending this, never leave it blank.
type buildTriggerRequest struct {
	RepoURL   string                   `json:"repo_url"`
	Ref       string                   `json:"ref"`
	ImageRepo string                   `json:"image_repo,omitempty"`
	Build     buildTriggerRequestBuild `json:"build,omitempty"`
}

// buildTriggerRequestBuild is triggerBuildRequest's nested build.* input
// (internal/api/builds.go's triggerBuildBuildInput), matching the same
// two fields app.yaml's own build: block has. Type left empty defaults
// server-side to a Dockerfile build, the only kind internal/deploy.Pipeline
// actually performs today.
type buildTriggerRequestBuild struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

// buildTriggerResponse mirrors internal/api's triggerBuildResponse: the
// real built image tag plus the app's full, now-updated resource (its
// desired_services.image row was overwritten by the build, per
// pendingImageTag's own doc comment above).
type buildTriggerResponse struct {
	Image string      `json:"image"`
	App   appResource `json:"app"`
}

// apiErrorBody is the JSON shape every non-2xx response from the
// control plane returns (internal/api/respond.go's own apiError).
type apiErrorBody struct {
	Error string `json:"error"`
}

// apiError is returned by Client methods for a non-2xx HTTP response: a
// real reply from the server, as opposed to a network-level failure
// (connection refused, DNS, timeout), which Client methods return as a
// plain wrapped error instead. Callers distinguish the two with
// errors.As, which is what picks the CLI's exit code.
type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
}

// Client is a minimal HTTP client for the control plane's versioned API
// (internal/api, mounted at /api/v1). It carries no session/cookie
// state: every request is a single bearer-token call, matching how this
// project's own token scheme (internal/api/abilities.go) is meant to be
// used by a non-interactive caller.
type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewClient builds a Client. baseURL should not have a trailing slash
// (trimmed defensively if it does). The timeout is long, not the usual
// few seconds a typical REST call would use: POST .../builds is
// synchronous and blocking (internal/api/builds.go's own doc comment),
// and a real Dockerfile build can legitimately take minutes.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		hc:      &http.Client{Timeout: 15 * time.Minute},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody) //nolint:gosec // c.baseURL is the operator-supplied --api-url/APP_API_URL target this whole command exists to call, not attacker-controlled input; that's the CLI's entire job, not an SSRF finding
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above, the request this CLI was invoked to make
	if err != nil {
		// A transport-level failure (connection refused, DNS, TLS,
		// timeout): deliberately not wrapped in *apiError, so callers
		// can tell "never reached the server" apart from "the server
		// answered with an error" via errors.As, and pick a different
		// exit code for each.
		return fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{StatusCode: resp.StatusCode, Message: extractErrorMessage(data)}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// extractErrorMessage parses the control plane's {"error": "..."} body
// shape (internal/api/respond.go). A body that doesn't match that shape
// (a proxy's HTML error page, an empty body) falls back to the raw text
// so the caller still sees something, not a blank message.
func extractErrorMessage(data []byte) string {
	var body apiErrorBody
	if err := json.Unmarshal(data, &body); err == nil && body.Error != "" {
		return body.Error
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// CreateApp calls POST /api/v1/apps.
func (c *Client) CreateApp(ctx context.Context, req appResource) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps", req, &out)
	return out, err
}

// GetApp calls GET /api/v1/apps/{name}.
func (c *Client) GetApp(ctx context.Context, name string) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+pathEscape(name), nil, &out)
	return out, err
}

// ListApps calls GET /api/v1/apps.
func (c *Client) ListApps(ctx context.Context) ([]appResource, error) {
	var out []appResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps", nil, &out)
	return out, err
}

// TriggerBuild calls POST /api/v1/apps/{name}/builds.
func (c *Client) TriggerBuild(ctx context.Context, name string, req buildTriggerRequest) (buildTriggerResponse, error) {
	var out buildTriggerResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+pathEscape(name)+"/builds", req, &out)
	return out, err
}

// pathEscape guards against a name containing characters that would
// otherwise change the request's URL shape (a "/" turning one path
// segment into two, for instance). Server-side validation is the real
// authority on what a valid app name is; this only protects the CLI's
// own request construction from building a request to the wrong path.
func pathEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), "/", "%2F")
}
