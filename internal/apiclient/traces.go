package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// TraceSummary mirrors internal/telemetry's TraceSummary.
type TraceSummary struct {
	TraceID    string `json:"trace_id"`
	RootName   string `json:"root_name"`
	StartNs    int64  `json:"start_ns"`
	DurationNs int64  `json:"duration_ns"`
	Spans      int    `json:"spans"`
	Errors     int    `json:"errors"`
}

// TraceSpan mirrors internal/api's spanResource.
type TraceSpan struct {
	TraceID       string          `json:"trace_id"`
	SpanID        string          `json:"span_id"`
	ParentID      string          `json:"parent_id,omitempty"`
	Name          string          `json:"name"`
	Kind          int             `json:"kind"`
	StartNs       int64           `json:"start_ns"`
	DurationNs    int64           `json:"duration_ns"`
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message,omitempty"`
	Attributes    json.RawMessage `json:"attributes,omitempty"`
}

// TracingResource mirrors internal/api's tracingResource.
type TracingResource struct {
	Enabled  bool              `json:"enabled"`
	Endpoint string            `json:"endpoint"`
	Header   string            `json:"header"`
	Key      string            `json:"key,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
}

// TraceListOptions are the filters of ListTraces.
type TraceListOptions struct {
	From       string
	To         string
	MinMillis  int
	ErrorsOnly bool
	Limit      int
}

func tracingPath(app string) string { return "/api/v1/apps/" + PathEscape(app) + "/tracing" }

// GetTracing calls GET /api/v1/apps/{name}/tracing.
func (c *Client) GetTracing(ctx context.Context, app string) (TracingResource, error) {
	var out TracingResource
	err := c.do(ctx, http.MethodGet, tracingPath(app), nil, &out)
	return out, err
}

// EnableTracing calls POST /api/v1/apps/{name}/tracing: creates or rotates
// the app's ingest key. The key is in the response only.
func (c *Client) EnableTracing(ctx context.Context, app string) (TracingResource, error) {
	var out TracingResource
	err := c.do(ctx, http.MethodPost, tracingPath(app), nil, &out)
	return out, err
}

// DisableTracing calls DELETE /api/v1/apps/{name}/tracing.
func (c *Client) DisableTracing(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, tracingPath(app), nil, nil)
}

// ListTraces calls GET /api/v1/apps/{name}/traces.
func (c *Client) ListTraces(ctx context.Context, app string, o TraceListOptions) ([]TraceSummary, error) {
	q := url.Values{}
	if o.From != "" {
		q.Set("from", o.From)
	}
	if o.To != "" {
		q.Set("to", o.To)
	}
	if o.MinMillis > 0 {
		q.Set("min_ms", strconv.Itoa(o.MinMillis))
	}
	if o.ErrorsOnly {
		q.Set("errors", "true")
	}
	if o.Limit > 0 {
		q.Set("limit", strconv.Itoa(o.Limit))
	}
	path := "/api/v1/apps/" + PathEscape(app) + "/traces"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out []TraceSummary
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetTrace calls GET /api/v1/apps/{name}/traces/{id}.
func (c *Client) GetTrace(ctx context.Context, app, traceID string) ([]TraceSpan, error) {
	var out []TraceSpan
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/traces/"+PathEscape(traceID), nil, &out)
	return out, err
}
