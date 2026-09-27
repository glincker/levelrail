package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AppMetricsSummary is one row of GET /api/v1/apps-metrics.
type AppMetricsSummary struct {
	Name         string     `json:"name"`
	CPUPercent   *float64   `json:"cpu_percent,omitempty"`
	MemoryBytes  *float64   `json:"memory_usage_bytes,omitempty"`
	MemoryLimit  *float64   `json:"memory_limit_bytes,omitempty"`
	HasTraffic   bool       `json:"has_traffic"`
	RatePerSec   float64    `json:"rate_per_sec"`
	ErrorRate5xx float64    `json:"error_rate_5xx"`
	P95Ms        float64    `json:"p95_ms"`
	Spark        []float64  `json:"spark"`
	LastDeployAt *time.Time `json:"last_deploy_at,omitempty"`
}

// ListAppMetricsSummary calls GET /api/v1/apps-metrics, optionally limited to names.
func (c *Client) ListAppMetricsSummary(ctx context.Context, names []string) ([]AppMetricsSummary, error) {
	path := "/api/v1/apps-metrics"
	if len(names) > 0 {
		q := url.Values{}
		q.Set("names", strings.Join(names, ","))
		path += "?" + q.Encode()
	}
	var out []AppMetricsSummary
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
