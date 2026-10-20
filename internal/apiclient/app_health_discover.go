package apiclient

import (
	"context"
	"net/http"
)

// HealthDiscoveryAttempt mirrors internal/api's healthDiscoveryAttempt
// (internal/api/apps_health_discover.go).
type HealthDiscoveryAttempt struct {
	Path      string `json:"path"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

// HealthDiscoveryResponse mirrors internal/api's healthDiscoveryResponse.
// Found is set only when exactly one candidate path succeeded.
type HealthDiscoveryResponse struct {
	Name     string                   `json:"name"`
	Attempts []HealthDiscoveryAttempt `json:"attempts"`
	Found    string                   `json:"found,omitempty"`
}

// DiscoverAppHealth calls POST /api/v1/apps/{name}/health/discover:
// actively probes name's running container against a fixed set of
// well-known paths and reports each one's real outcome. Never guesses:
// every path in the response was actually tried.
func (c *Client) DiscoverAppHealth(ctx context.Context, name string) (HealthDiscoveryResponse, error) {
	var out HealthDiscoveryResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/health/discover", nil, &out)
	return out, err
}
