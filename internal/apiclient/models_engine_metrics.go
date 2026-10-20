package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// EngineMetricPoint is one stored engine reading.
type EngineMetricPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// EngineMetricSeries mirrors internal/models.EngineMetricSeries.
type EngineMetricSeries struct {
	ID        string              `json:"id"`
	Label     string              `json:"label"`
	Unit      string              `json:"unit"`
	Supported bool                `json:"supported"`
	Latest    *float64            `json:"latest"`
	Points    []EngineMetricPoint `json:"points"`
}

// EngineHealth mirrors internal/models.EngineHealth.
type EngineHealth struct {
	State   string   `json:"state"`
	Summary string   `json:"summary"`
	Reasons []string `json:"reasons"`
}

// EngineMetricsReport mirrors internal/models.EngineMetricsReport.
type EngineMetricsReport struct {
	Model      string               `json:"model"`
	Engine     string               `json:"engine"`
	From       time.Time            `json:"from"`
	To         time.Time            `json:"to"`
	Collecting bool                 `json:"collecting"`
	Note       string               `json:"note"`
	Health     EngineHealth         `json:"health"`
	Series     []EngineMetricSeries `json:"series"`
}

// GetModelEngineMetrics calls GET /api/v1/models/{name}/engine-metrics.
func (c *Client) GetModelEngineMetrics(ctx context.Context, name string, since time.Duration) (EngineMetricsReport, error) {
	q := url.Values{}
	if since > 0 {
		q.Set("since", since.String())
	}
	var out EngineMetricsReport
	err := c.do(ctx, http.MethodGet, "/api/v1/models/"+PathEscape(name)+"/engine-metrics?"+q.Encode(), nil, &out)
	return out, err
}
