package apiclient

import (
	"context"
	"net/http"
)

// ProbeAttempt is one readiness-probe attempt made during a deploy cutover.
type ProbeAttempt struct {
	ID         int64  `json:"id"`
	Target     string `json:"target"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	ExitCode   int    `json:"exit_code,omitempty"`
	Error      string `json:"error,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	ProbedAt   string `json:"probed_at"`
}

// ListProbeAttempts calls GET /api/v1/apps/{name}/deploys/{deployID}/probes.
func (c *Client) ListProbeAttempts(ctx context.Context, name, deployID string) ([]ProbeAttempt, error) {
	var out []ProbeAttempt
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/deploys/"+PathEscape(deployID)+"/probes", nil, &out)
	return out, err
}
