package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MonitorRange is an inclusive HTTP status interval.
type MonitorRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Monitor mirrors internal/api's monitorResource and monitorRequest.
type Monitor struct {
	ID                  string            `json:"id,omitempty"`
	Name                string            `json:"name"`
	Kind                string            `json:"kind"`
	IntervalSeconds     int               `json:"interval_seconds"`
	TimeoutMS           int               `json:"timeout_ms"`
	Enabled             *bool             `json:"enabled,omitempty"`
	URL                 string            `json:"url,omitempty"`
	Method              string            `json:"method,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	HeaderNames         []string          `json:"header_names,omitempty"`
	Expect              []MonitorRange    `json:"expect,omitempty"`
	Contains            string            `json:"contains,omitempty"`
	FollowRedirects     bool              `json:"follow_redirects,omitempty"`
	Host                string            `json:"host,omitempty"`
	Port                int               `json:"port,omitempty"`
	StatusLink          bool              `json:"status_link,omitempty"`
	LinkConfirmFailures int               `json:"link_confirm_failures,omitempty"`
	LinkRecoverySeconds int               `json:"link_recovery_seconds,omitempty"`
	GraceSeconds        int               `json:"grace_seconds,omitempty"`
	WarnDays            int               `json:"warn_days,omitempty"`
	CritDays            int               `json:"crit_days,omitempty"`
	PingPath            string            `json:"ping_path,omitempty"`
	CreatedAt           time.Time         `json:"created_at,omitempty"`
}

// MonitorResult is one raw probe outcome.
type MonitorResult struct {
	At        int64  `json:"at"`
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latency_ms"`
	Code      int    `json:"code,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// MonitorRollup is one hour of results.
type MonitorRollup struct {
	Bucket       int64 `json:"bucket"`
	Total        int   `json:"total"`
	Failed       int   `json:"failed"`
	AvgLatencyMS int   `json:"avg_latency_ms"`
	MaxLatencyMS int   `json:"max_latency_ms"`
}

// MonitorResults is the body of GET /api/v1/monitors/{id}/results.
type MonitorResults struct {
	MonitorID string          `json:"monitor_id"`
	Raw       []MonitorResult `json:"raw"`
	Rollup    []MonitorRollup `json:"rollup"`
}

// ListMonitors calls GET /api/v1/monitors.
func (c *Client) ListMonitors(ctx context.Context) ([]Monitor, error) {
	var out []Monitor
	err := c.do(ctx, http.MethodGet, "/api/v1/monitors", nil, &out)
	return out, err
}

// CreateMonitor calls POST /api/v1/monitors.
func (c *Client) CreateMonitor(ctx context.Context, m Monitor) (Monitor, error) {
	var out Monitor
	err := c.do(ctx, http.MethodPost, "/api/v1/monitors", m, &out)
	return out, err
}

// MonitorLink is the body of PATCH /api/v1/monitors/{id}.
type MonitorLink struct {
	StatusLink          bool `json:"status_link"`
	LinkConfirmFailures int  `json:"link_confirm_failures"`
	LinkRecoverySeconds int  `json:"link_recovery_seconds"`
}

// UpdateMonitorLink calls PATCH /api/v1/monitors/{id}.
func (c *Client) UpdateMonitorLink(ctx context.Context, id string, l MonitorLink) (Monitor, error) {
	var out Monitor
	err := c.do(ctx, http.MethodPatch, "/api/v1/monitors/"+PathEscape(id), l, &out)
	return out, err
}

// RotateMonitorToken calls POST /api/v1/monitors/{id}/rotate-token and returns the new ping path once.
func (c *Client) RotateMonitorToken(ctx context.Context, id string) (Monitor, error) {
	var out Monitor
	err := c.do(ctx, http.MethodPost, "/api/v1/monitors/"+PathEscape(id)+"/rotate-token", nil, &out)
	return out, err
}

// DeleteMonitor calls DELETE /api/v1/monitors/{id}.
func (c *Client) DeleteMonitor(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/monitors/"+PathEscape(id), nil, nil)
}

// MonitorResultsFor calls GET /api/v1/monitors/{id}/results.
func (c *Client) MonitorResultsFor(ctx context.Context, id, window string, limit int) (MonitorResults, error) {
	q := url.Values{}
	if window != "" {
		q.Set("window", window)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/monitors/" + PathEscape(id) + "/results"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out MonitorResults
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
