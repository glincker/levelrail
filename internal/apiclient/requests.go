package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// RequestPointResource mirrors internal/telemetry.RequestPoint.
type RequestPointResource struct {
	Timestamp      time.Time `json:"timestamp"`
	Requests       float64   `json:"requests"`
	RatePerSec     float64   `json:"rate_per_sec"`
	ErrorRate4xx   float64   `json:"error_rate_4xx"`
	ErrorRate5xx   float64   `json:"error_rate_5xx"`
	UpstreamErrors float64   `json:"upstream_errors"`
	P50Ms          float64   `json:"p50_ms"`
	P95Ms          float64   `json:"p95_ms"`
	P99Ms          float64   `json:"p99_ms"`
	BytesInPerSec  float64   `json:"bytes_in_per_sec"`
	BytesOutPerSec float64   `json:"bytes_out_per_sec"`
}

// RequestSummaryResource mirrors internal/telemetry.RequestSummary.
type RequestSummaryResource struct {
	WindowSeconds  float64 `json:"window_seconds"`
	HasTraffic     bool    `json:"has_traffic"`
	Requests       float64 `json:"requests"`
	RatePerSec     float64 `json:"rate_per_sec"`
	ErrorRate4xx   float64 `json:"error_rate_4xx"`
	ErrorRate5xx   float64 `json:"error_rate_5xx"`
	P95Ms          float64 `json:"p95_ms"`
	UpstreamErrors float64 `json:"upstream_errors"`
}

// AppRequestsResource mirrors internal/api's requestsResponse.
type AppRequestsResource struct {
	App         string                 `json:"app"`
	From        time.Time              `json:"from"`
	To          time.Time              `json:"to"`
	StepSeconds float64                `json:"step_seconds"`
	Summary     RequestSummaryResource `json:"summary"`
	Points      []RequestPointResource `json:"points"`
}

// QueryAppRequests calls GET /api/v1/apps/{name}/requests: ingress request
// rate, error rates and latency percentiles. A zero step lets the server pick.
func (c *Client) QueryAppRequests(ctx context.Context, name string, from, to time.Time, step time.Duration) (AppRequestsResource, error) {
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if step > 0 {
		query.Set("step", step.String())
	}
	var out AppRequestsResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/requests?"+query.Encode(), nil, &out)
	return out, err
}
