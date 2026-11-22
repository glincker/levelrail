package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
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
	return getJSON[ErrorTrackingResource](ctx, c, errorTrackingPath(app))
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
	return c.del(ctx, errorTrackingPath(app))
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
	return getJSON[[]ErrorIssue](ctx, c, path)
}

// GetError calls GET /api/v1/apps/{name}/errors/{fingerprint}.
func (c *Client) GetError(ctx context.Context, app, fingerprint string) (ErrorIssueDetail, error) {
	return getJSON[ErrorIssueDetail](ctx, c, "/api/v1/apps/"+PathEscape(app)+"/errors/"+PathEscape(fingerprint))
}

// SetErrorStatus calls PATCH /api/v1/apps/{name}/errors/{fingerprint}.
func (c *Client) SetErrorStatus(ctx context.Context, app, fingerprint, status string) (ErrorIssueDetail, error) {
	var out ErrorIssueDetail
	body := map[string]string{"status": status}
	err := c.do(ctx, http.MethodPatch, "/api/v1/apps/"+PathEscape(app)+"/errors/"+PathEscape(fingerprint), body, &out)
	return out, err
}
