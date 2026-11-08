package apiclient

import (
	"context"
	"net/http"
)

// NextAdvisorFinding is one check result from the Next.js advisor.
type NextAdvisorFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
}

// NextAdvisorResult is the response of GET /api/v1/apps/{name}/next-advisor.
type NextAdvisorResult struct {
	Detected bool                 `json:"detected"`
	Findings []NextAdvisorFinding `json:"findings"`
}

// NextBuildRoute is one row of the Next.js build route table.
type NextBuildRoute struct {
	Path        string  `json:"path"`
	Kind        string  `json:"kind"`
	SizeKB      float64 `json:"size_kb"`
	FirstLoadKB float64 `json:"first_load_kb"`
}

// NextBuildReport is the response of GET /api/v1/apps/{name}/deploys/{id}/build-report.
type NextBuildReport struct {
	Detected        bool             `json:"detected"`
	Routes          []NextBuildRoute `json:"routes"`
	SharedFirstLoad *float64         `json:"shared_first_load_kb"`
	Warnings        []string         `json:"warnings"`
}

// NextAdvisor returns the Next.js checks of an app's saved config.
func (c *Client) NextAdvisor(ctx context.Context, app string) (NextAdvisorResult, error) {
	var out NextAdvisorResult
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/next-advisor", nil, &out)
	return out, err
}

// NextBuildReport returns the route table and warnings parsed from one deploy's build log.
func (c *Client) NextBuildReport(ctx context.Context, app, deployID string) (NextBuildReport, error) {
	var out NextBuildReport
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/deploys/"+PathEscape(deployID)+"/build-report", nil, &out)
	return out, err
}
