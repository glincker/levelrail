package apiclient

import (
	"context"
	"net/http"
	"time"
)

// OutageCheck is one survival property in a resilience report.
type OutageCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// OutageScope groups the checks of the ingress or one node.
type OutageScope struct {
	Kind   string        `json:"kind"`
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Checks []OutageCheck `json:"checks"`
}

// OutageReport is GET /api/v1/system/resilience.
type OutageReport struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Simulated   bool          `json:"simulated"`
	Overall     string        `json:"overall"`
	Scopes      []OutageScope `json:"scopes"`
}

// GetResilience calls GET /api/v1/system/resilience.
func (c *Client) GetResilience(ctx context.Context) (OutageReport, error) {
	var out OutageReport
	err := c.do(ctx, http.MethodGet, "/api/v1/system/resilience", nil, &out)
	return out, err
}

// SimulateResilience calls POST /api/v1/system/resilience/simulate (a dry run, nothing is stopped).
func (c *Client) SimulateResilience(ctx context.Context) (OutageReport, error) {
	var out OutageReport
	err := c.do(ctx, http.MethodPost, "/api/v1/system/resilience/simulate", nil, &out)
	return out, err
}

// FixResilienceSnapshot calls POST /api/v1/system/resilience/snapshot and returns the re-checked report.
func (c *Client) FixResilienceSnapshot(ctx context.Context) (OutageReport, error) {
	var out OutageReport
	err := c.do(ctx, http.MethodPost, "/api/v1/system/resilience/snapshot", nil, &out)
	return out, err
}
