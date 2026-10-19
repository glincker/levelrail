package apiclient

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

// ResilienceReport mirrors GET /api/v1/resilience.
type ResilienceReport struct {
	Scenarios  []chaos.Info   `json:"scenarios"`
	Latest     []chaos.Result `json:"latest"`
	Unmeasured []string       `json:"not_yet_measured"`
}

// GetResilienceReport calls GET /api/v1/resilience.
func (c *Client) GetResilienceReport(ctx context.Context) (ResilienceReport, error) {
	var out ResilienceReport
	err := c.do(ctx, http.MethodGet, "/api/v1/resilience", nil, &out)
	return out, err
}

// RecordChaosResults calls POST /api/v1/resilience/results and returns the stored IDs.
func (c *Client) RecordChaosResults(ctx context.Context, results []chaos.Result) ([]string, error) {
	var out struct {
		IDs []string `json:"ids"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/resilience/results", map[string][]chaos.Result{"results": results}, &out)
	return out.IDs, err
}
