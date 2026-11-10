package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// AppScrapeTargetResource is GET/PUT /api/v1/apps/{name}/scrape-target.
type AppScrapeTargetResource struct {
	Configured bool   `json:"configured"`
	Port       int    `json:"port"`
	Path       string `json:"path"`
	LastError  string `json:"last_error,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// AppScrapedPoint is one stored sample of a scraped series.
type AppScrapedPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// AppScrapedSeries is one label set of a scraped metric.
type AppScrapedSeries struct {
	Labels string            `json:"labels"`
	Points []AppScrapedPoint `json:"points"`
}

// AppScrapedMetricsResource is GET /api/v1/apps/{name}/scraped-metrics.
type AppScrapedMetricsResource struct {
	Names  []string           `json:"names,omitempty"`
	Metric string             `json:"metric,omitempty"`
	Series []AppScrapedSeries `json:"series,omitempty"`
}

// GetAppScrapeTarget reads where an app's own metrics are scraped from.
func (c *Client) GetAppScrapeTarget(ctx context.Context, name string) (AppScrapeTargetResource, error) {
	var out AppScrapeTargetResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/scrape-target", nil, &out)
	return out, err
}

// SetAppScrapeTarget sets the container port and path the control plane scrapes.
func (c *Client) SetAppScrapeTarget(ctx context.Context, name string, port int, path string) (AppScrapeTargetResource, error) {
	var out AppScrapeTargetResource
	body := map[string]any{"port": port, "path": path}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/scrape-target", body, &out)
	return out, err
}

// DeleteAppScrapeTarget stops scraping an app.
func (c *Client) DeleteAppScrapeTarget(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/scrape-target", nil, nil)
}

// QueryAppScrapedMetrics lists scraped names when metric is empty, else one metric's series.
func (c *Client) QueryAppScrapedMetrics(ctx context.Context, name, metric string, from, to time.Time) (AppScrapedMetricsResource, error) {
	q := url.Values{}
	if metric != "" {
		q.Set("metric", metric)
		q.Set("from", from.UTC().Format(time.RFC3339))
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/scraped-metrics"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out AppScrapedMetricsResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
