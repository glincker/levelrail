package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrorIssue mirrors internal/telemetry's ErrorIssue.
type ErrorIssue struct {
	Fingerprint  string `json:"fingerprint"`
	Title        string `json:"title"`
	Level        string `json:"level"`
	FirstSeenNs  int64  `json:"first_seen_ns"`
	LastSeenNs   int64  `json:"last_seen_ns"`
	Count        int64  `json:"count"`
	LastRelease  string `json:"last_release,omitempty"`
	LastDeployID string `json:"last_deploy_id,omitempty"`
	Status       string `json:"status"`
	Regressed    bool   `json:"regressed"`
}

// ErrorEvent mirrors internal/api's errorEventResource.
type ErrorEvent struct {
	EventID     string          `json:"event_id"`
	TimestampNs int64           `json:"timestamp_ns"`
	Release     string          `json:"release,omitempty"`
	Environment string          `json:"environment,omitempty"`
	DeployID    string          `json:"deploy_id,omitempty"`
	Event       json.RawMessage `json:"event,omitempty"`
}

// ErrorIssueDetail mirrors internal/api's errorIssueResource.
type ErrorIssueDetail struct {
	Issue  ErrorIssue   `json:"issue"`
	Events []ErrorEvent `json:"events"`
}

// ErrorTrackingResource mirrors internal/api's errorTrackingResource.
type ErrorTrackingResource struct {
	Enabled  bool              `json:"enabled"`
	Endpoint string            `json:"endpoint"`
	DSN      string            `json:"dsn,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}

// ErrorListOptions are the filters of ListErrors.
type ErrorListOptions struct {
	Status string
	Since  string
	Limit  int
}

func errorTrackingPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/error-tracking"
}

// GetErrorTracking calls GET /api/v1/apps/{name}/error-tracking.
func (c *Client) GetErrorTracking(ctx context.Context, app string) (ErrorTrackingResource, error) {
	var out ErrorTrackingResource
	err := c.do(ctx, http.MethodGet, errorTrackingPath(app), nil, &out)
	return out, err
}

// EnableErrorTracking calls POST /api/v1/apps/{name}/error-tracking: creates
// or rotates the DSN key, which is in the response only.
func (c *Client) EnableErrorTracking(ctx context.Context, app string) (ErrorTrackingResource, error) {
	var out ErrorTrackingResource
	err := c.do(ctx, http.MethodPost, errorTrackingPath(app), nil, &out)
	return out, err
}

// DisableErrorTracking calls DELETE /api/v1/apps/{name}/error-tracking.
func (c *Client) DisableErrorTracking(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, errorTrackingPath(app), nil, nil)
}

// ListErrors calls GET /api/v1/apps/{name}/errors.
func (c *Client) ListErrors(ctx context.Context, app string, o ErrorListOptions) ([]ErrorIssue, error) {
	q := url.Values{}
	if o.Status != "" {
		q.Set("status", o.Status)
	}
	if o.Since != "" {
		q.Set("since", o.Since)
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/errors"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []ErrorIssue
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetError calls GET /api/v1/apps/{name}/errors/{fingerprint}.
func (c *Client) GetError(ctx context.Context, app, fingerprint string) (ErrorIssueDetail, error) {
	var out ErrorIssueDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/errors/"+PathEscape(fingerprint), nil, &out)
	return out, err
}

// SetErrorStatus calls PATCH /api/v1/apps/{name}/errors/{fingerprint}.
func (c *Client) SetErrorStatus(ctx context.Context, app, fingerprint, status string) (ErrorIssueDetail, error) {
	var out ErrorIssueDetail
	body := map[string]string{"status": status}
	err := c.do(ctx, http.MethodPatch, "/api/v1/apps/"+PathEscape(app)+"/errors/"+PathEscape(fingerprint), body, &out)
	return out, err
}

// NotSupportedError says the server answered like an older build that lacks an endpoint.
// It wraps the APIError, so exit code classification stays the same.
type NotSupportedError struct {
	Feature string
	Cause   *APIError
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("this server does not support %s yet (HTTP %d); upgrade the server or use a CLI that matches it", e.Feature, e.Cause.StatusCode)
}

func (e *NotSupportedError) Unwrap() error { return e.Cause }

// NotSupported turns a missing-route answer into a NotSupportedError. A 404 that names a
// missing resource (app, database) is left alone, since that is the caller's mistake.
func NotSupported(feature string, err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	switch apiErr.StatusCode {
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return &NotSupportedError{Feature: feature, Cause: apiErr}
	case http.StatusNotFound:
		switch strings.ToLower(strings.TrimSpace(apiErr.Message)) {
		case "", "404 page not found", "page not found", "not found":
			return &NotSupportedError{Feature: feature, Cause: apiErr}
		}
	}
	return err
}

// IsNotSupported reports whether err is a NotSupportedError.
func IsNotSupported(err error) bool {
	var ns *NotSupportedError
	return errors.As(err, &ns)
}
