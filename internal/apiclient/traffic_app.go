package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// TrafficLatency is a latency percentile triple in milliseconds.
type TrafficLatency struct {
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

// TrafficStatusClasses counts responses by status class.
type TrafficStatusClasses struct {
	Status2xx float64 `json:"2xx"`
	Status3xx float64 `json:"3xx"`
	Status4xx float64 `json:"4xx"`
	Status5xx float64 `json:"5xx"`
}

// TrafficReplica is one upstream's RED figures.
type TrafficReplica struct {
	Upstream       string  `json:"upstream"`
	Requests       float64 `json:"requests"`
	RatePerSec     float64 `json:"rate_per_sec"`
	ErrorRate4xx   float64 `json:"error_rate_4xx"`
	ErrorRate5xx   float64 `json:"error_rate_5xx"`
	UpstreamErrors float64 `json:"upstream_errors"`
	TrafficLatency
}

// TrafficRoute is one normalized route template's totals.
type TrafficRoute struct {
	Route        string  `json:"route"`
	Requests     float64 `json:"requests"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	AvgMs        float64 `json:"avg_ms"`
}

// TrafficSample is one slow or failed request.
type TrafficSample struct {
	Time       time.Time `json:"time"`
	Route      string    `json:"route"`
	Method     string    `json:"method"`
	Status     int       `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	Upstream   string    `json:"upstream,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
}

// TrafficLabel is one referrer host or country with its request share.
type TrafficLabel struct {
	Label    string  `json:"label"`
	Requests float64 `json:"requests"`
	Share    float64 `json:"share"`
}

// TrafficSLO is the app's SLO error-budget burn over the window.
type TrafficSLO struct {
	Objective       string  `json:"objective"`
	Target          float64 `json:"target"`
	LatencyMs       float64 `json:"latency_ms,omitempty"`
	BurnRate        float64 `json:"burn_rate"`
	BudgetRemaining float64 `json:"budget_remaining"`
}

// AppTrafficResource is GET /api/v1/apps/{name}/traffic.
type AppTrafficResource struct {
	App            string               `json:"app"`
	Window         string               `json:"window"`
	From           time.Time            `json:"from"`
	To             time.Time            `json:"to"`
	HasTraffic     bool                 `json:"has_traffic"`
	Requests       float64              `json:"requests"`
	RatePerSec     float64              `json:"rate_per_sec"`
	StatusClasses  TrafficStatusClasses `json:"status_classes"`
	ErrorRate4xx   float64              `json:"error_rate_4xx"`
	ErrorRate5xx   float64              `json:"error_rate_5xx"`
	UpstreamErrors float64              `json:"upstream_errors"`
	Latency        TrafficLatency       `json:"latency"`
	Replicas       []TrafficReplica     `json:"replicas"`
	SlowRoutes     []TrafficRoute       `json:"slow_routes"`
	TopRoutes      []TrafficRoute       `json:"top_routes"`
	Samples        []TrafficSample      `json:"samples"`
	Referrers      []TrafficLabel       `json:"referrers"`
	Countries      []TrafficLabel       `json:"countries"`
	SLO            *TrafficSLO          `json:"slo,omitempty"`
	Shield         TrafficShield        `json:"shield"`
}

// GetAppTraffic calls GET /api/v1/apps/{name}/traffic for window 5m, 1h, 24h or 7d.
func (c *Client) GetAppTraffic(ctx context.Context, name, window string) (AppTrafficResource, error) {
	query := url.Values{}
	if window != "" {
		query.Set("window", window)
	}
	var out AppTrafficResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/traffic?"+query.Encode(), nil, &out)
	return out, err
}
