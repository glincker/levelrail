package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// CDNOverview mirrors GET /api/v1/cdn/overview.
type CDNOverview struct {
	Window         string    `json:"window"`
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	HasTraffic     bool      `json:"has_traffic"`
	Requests       float64   `json:"requests"`
	RatePerSec     float64   `json:"rate_per_sec"`
	ErrorRate5xx   float64   `json:"error_rate_5xx"`
	BytesOut       float64   `json:"bytes_out"`
	Cache          CDNCache  `json:"cache"`
	Nodes          []CDNNode `json:"nodes"`
	RegionsLabeled bool      `json:"regions_labeled"`
}

// CDNCache is the edge cache part of CDNOverview.
type CDNCache struct {
	CoveredMinutes int     `json:"covered_minutes"`
	Hits           uint64  `json:"hits"`
	Misses         uint64  `json:"misses"`
	HitRate        float64 `json:"hit_rate"`
	HitBytes       uint64  `json:"hit_bytes"`
	OriginBytes    uint64  `json:"origin_bytes"`
}

// CDNNode is one node's share of traffic in CDNOverview.
type CDNNode struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Region   string  `json:"region,omitempty"`
	Requests float64 `json:"requests"`
	Share    float64 `json:"share"`
	Apps     int     `json:"apps"`
}

// GetCDNOverview calls GET /api/v1/cdn/overview, window being 5m, 1h, 24h or 7d (empty means 1h).
func (c *Client) GetCDNOverview(ctx context.Context, window string) (CDNOverview, error) {
	path := "/api/v1/cdn/overview"
	if window != "" {
		path += "?window=" + url.QueryEscape(window)
	}
	var out CDNOverview
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
