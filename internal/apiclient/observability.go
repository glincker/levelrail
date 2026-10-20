package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MetricsQueryOptions are the optional downsampling and comparison controls
// of GET /api/v1/apps/{name}/metrics.
type MetricsQueryOptions struct {
	Step      time.Duration
	MaxPoints int
	Compare   bool
}

// MetricsSeriesResource mirrors internal/api's metricsResponse.
type MetricsSeriesResource struct {
	Metric         string              `json:"metric"`
	StepSeconds    float64             `json:"step_seconds"`
	Downsampled    bool                `json:"downsampled"`
	Points         []MetricSeriesPoint `json:"points"`
	PreviousPoints []MetricSeriesPoint `json:"previous_points,omitempty"`
}

// MetricSeriesPoint is one bucket; Max keeps spikes visible after averaging.
type MetricSeriesPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Count     int       `json:"count"`
	Max       float64   `json:"max"`
}

// QueryAppMetricsSeries calls GET /api/v1/apps/{name}/metrics with max_points
// and compare support.
func (c *Client) QueryAppMetricsSeries(ctx context.Context, name, metric string, from, to time.Time, opt MetricsQueryOptions) (MetricsSeriesResource, error) {
	q := url.Values{}
	q.Set("metric", metric)
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	if opt.Step > 0 {
		q.Set("step", opt.Step.String())
	}
	if opt.MaxPoints > 0 {
		q.Set("max_points", strconv.Itoa(opt.MaxPoints))
	}
	if opt.Compare {
		q.Set("compare", "previous")
	}
	var out MetricsSeriesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/metrics?"+q.Encode(), nil, &out)
	return out, err
}

// InvestigateSummary is one window's request health.
type InvestigateSummary struct {
	HasTraffic   bool    `json:"has_traffic"`
	Requests     float64 `json:"requests"`
	RatePerSec   float64 `json:"rate_per_sec"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
}

// InvestigateRoute is one route's share of a window.
type InvestigateRoute struct {
	Route        string  `json:"route"`
	Requests     float64 `json:"requests"`
	Share        float64 `json:"share"`
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	AvgMs        float64 `json:"avg_ms"`
}

// InvestigateStatus is one exact status code's count.
type InvestigateStatus struct {
	Status int     `json:"status"`
	Count  float64 `json:"count"`
	Share  float64 `json:"share"`
}

// InvestigateEvent is one What changed timeline entry.
type InvestigateEvent struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail,omitempty"`
	Ref         string    `json:"ref,omitempty"`
	LikelyCause bool      `json:"likely_cause,omitempty"`
}

// InvestigationResource mirrors internal/api's investigateResponse.
type InvestigationResource struct {
	App             string              `json:"app"`
	From            time.Time           `json:"from"`
	To              time.Time           `json:"to"`
	Summary         InvestigateSummary  `json:"summary"`
	Baseline        InvestigateSummary  `json:"baseline"`
	RoutesAvailable bool                `json:"routes_available"`
	TopRoutes       []InvestigateRoute  `json:"top_routes"`
	StatusCodes     []InvestigateStatus `json:"status_codes"`
	Timeline        []InvestigateEvent  `json:"timeline"`
}

// InvestigateApp calls GET /api/v1/apps/{name}/investigate.
func (c *Client) InvestigateApp(ctx context.Context, name string, from, to time.Time) (InvestigationResource, error) {
	q := url.Values{}
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	var out InvestigationResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/investigate?"+q.Encode(), nil, &out)
	return out, err
}

// FailureContextLine is one log line of a failure context.
type FailureContextLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Level     string    `json:"level,omitempty"`
}

// FailureContextDeploy is the failed deploy attempt, when there is one.
type FailureContextDeploy struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Image      string     `json:"image"`
	Commit     string     `json:"commit,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// FailureContextResource mirrors internal/api's failureContextResponse.
type FailureContextResource struct {
	State         string                `json:"state"`
	Since         *time.Time            `json:"since,omitempty"`
	Restarts      int                   `json:"restarts_in_window"`
	WindowSeconds float64               `json:"window_seconds"`
	ContainerID   string                `json:"container_id,omitempty"`
	Deploy        *FailureContextDeploy `json:"deploy,omitempty"`
	Lines         []FailureContextLine  `json:"lines"`
	TotalLines    int                   `json:"total_lines"`
	LinesSource   string                `json:"lines_source,omitempty"`
}

// GetFailureContext calls GET /api/v1/apps/{name}/failure-context.
func (c *Client) GetFailureContext(ctx context.Context, name string) (FailureContextResource, error) {
	var out FailureContextResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/failure-context", nil, &out)
	return out, err
}

// LogSearch selects stored log lines with the structured filters.
type LogSearch struct {
	Text      string
	Level     string
	Container string
	Stream    string
	Fields    []string
	From      time.Time
	To        time.Time
	Limit     int
}

// LogSearchEntry is one stored log line.
type LogSearchEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	Level     string    `json:"level,omitempty"`
	Container string    `json:"container,omitempty"`
}

// LogSearchContainer is a container seen in a search, with its line count.
type LogSearchContainer struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// LogSearchResource mirrors internal/api's logsResponse.
type LogSearchResource struct {
	Entries    []LogSearchEntry     `json:"entries"`
	Total      int                  `json:"total"`
	Containers []LogSearchContainer `json:"containers"`
}

// SearchAppLogs calls GET /api/v1/apps/{name}/logs with the filter params.
func (c *Client) SearchAppLogs(ctx context.Context, name string, s LogSearch) (LogSearchResource, error) {
	q := url.Values{}
	q.Set("from", s.From.UTC().Format(time.RFC3339))
	q.Set("to", s.To.UTC().Format(time.RFC3339))
	set := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	set("q", s.Text)
	set("level", s.Level)
	set("container", s.Container)
	set("stream", s.Stream)
	for _, f := range s.Fields {
		q.Add("field", f)
	}
	if s.Limit > 0 {
		q.Set("limit", strconv.Itoa(s.Limit))
	}
	var out LogSearchResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/logs?"+q.Encode(), nil, &out)
	return out, err
}
