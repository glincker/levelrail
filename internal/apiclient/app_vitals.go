package apiclient

import (
	"context"
	"net/http"
)

// AppVitalsStat mirrors internal/api's vitalsStat.
type AppVitalsStat struct {
	Count  int64   `json:"count"`
	P50    float64 `json:"p50"`
	P75    float64 `json:"p75"`
	P95    float64 `json:"p95"`
	Rating string  `json:"rating"`
}

// AppVitalsRoute mirrors internal/api's vitalsRouteRow.
type AppVitalsRoute struct {
	Route   string                   `json:"route"`
	Metrics map[string]AppVitalsStat `json:"metrics"`
}

// AppVitalsResource mirrors internal/api's vitalsResponse.
type AppVitalsResource struct {
	Enabled   bool                     `json:"enabled"`
	PublicKey string                   `json:"public_key"`
	Days      int                      `json:"days"`
	Overall   map[string]AppVitalsStat `json:"overall"`
	Routes    []AppVitalsRoute         `json:"routes"`
}

// GetAppVitals calls GET /api/v1/apps/{name}/vitals: p50, p75 and p95 per metric and route over the report window.
func (c *Client) GetAppVitals(ctx context.Context, name string) (AppVitalsResource, error) {
	var out AppVitalsResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/vitals", nil, &out)
	return out, err
}
