package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// TrafficExportSink mirrors one sink of /api/v1/settings/traffic-export/sinks. URL, Token and AuthHeader are
// write-only: a response carries only the *Set booleans, and an empty value on a write keeps the stored one.
type TrafficExportSink struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Enabled    *bool    `json:"enabled,omitempty"`
	Host       string   `json:"host,omitempty"`
	TargetHost string   `json:"target_host,omitempty"`
	Fields     []string `json:"fields"`
	IPTruncate *bool    `json:"ip_truncate,omitempty"`
	Sample     *float64 `json:"sample,omitempty"`

	URL           string `json:"url,omitempty"`
	URLSet        bool   `json:"url_set,omitempty"`
	Token         string `json:"token,omitempty"`
	TokenSet      bool   `json:"token_set,omitempty"`
	AuthHeader    string `json:"auth_header,omitempty"`
	AuthHeaderSet bool   `json:"auth_header_set,omitempty"`

	IdentityEnabled bool       `json:"identity_enabled"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

// TrafficExportSinkList mirrors GET /api/v1/settings/traffic-export/sinks.
type TrafficExportSinkList struct {
	Sinks []TrafficExportSink `json:"sinks"`
}

// TrafficExportSinkStatus is the live view of one sink.
type TrafficExportSinkStatus struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Enabled       bool      `json:"enabled"`
	Active        bool      `json:"active"`
	LastSuccess   time.Time `json:"last_success"`
	LastErrorKind string    `json:"last_error_kind,omitempty"`
	Queued        int       `json:"queued"`
	Sent          uint64    `json:"sent"`
	Dropped       uint64    `json:"dropped"`
	Failed        uint64    `json:"failed"`
	SampledOut    uint64    `json:"sampled_out"`
}

// TrafficExportStatus mirrors GET /api/v1/settings/traffic-export/status.
type TrafficExportStatus struct {
	Sinks []TrafficExportSinkStatus `json:"sinks"`
}

// TrafficExportTestResult mirrors the test-delivery response.
type TrafficExportTestResult struct {
	OK         bool    `json:"ok"`
	ErrorKind  string  `json:"error_kind,omitempty"`
	DurationMS float64 `json:"duration_ms"`
}

const trafficExportBase = "/api/v1/settings/traffic-export"

func trafficExportSinkPath(id string) string {
	return trafficExportBase + "/sinks/" + url.PathEscape(id)
}

// ListTrafficExportSinks calls GET /api/v1/settings/traffic-export/sinks.
func (c *Client) ListTrafficExportSinks(ctx context.Context) (TrafficExportSinkList, error) {
	var out TrafficExportSinkList
	err := c.do(ctx, http.MethodGet, trafficExportBase+"/sinks", nil, &out)
	return out, err
}

// GetTrafficExportSink calls GET /api/v1/settings/traffic-export/sinks/{id}.
func (c *Client) GetTrafficExportSink(ctx context.Context, id string) (TrafficExportSink, error) {
	var out TrafficExportSink
	err := c.do(ctx, http.MethodGet, trafficExportSinkPath(id), nil, &out)
	return out, err
}

// CreateTrafficExportSink calls POST /api/v1/settings/traffic-export/sinks.
func (c *Client) CreateTrafficExportSink(ctx context.Context, req TrafficExportSink) (TrafficExportSink, error) {
	var out TrafficExportSink
	err := c.do(ctx, http.MethodPost, trafficExportBase+"/sinks", req, &out)
	return out, err
}

// UpdateTrafficExportSink calls PUT /api/v1/settings/traffic-export/sinks/{id}.
func (c *Client) UpdateTrafficExportSink(ctx context.Context, id string, req TrafficExportSink) (TrafficExportSink, error) {
	var out TrafficExportSink
	err := c.do(ctx, http.MethodPut, trafficExportSinkPath(id), req, &out)
	return out, err
}

// DeleteTrafficExportSink calls DELETE /api/v1/settings/traffic-export/sinks/{id}.
func (c *Client) DeleteTrafficExportSink(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, trafficExportSinkPath(id), nil, nil)
}

// TestTrafficExportSink calls POST /api/v1/settings/traffic-export/sinks/{id}/test.
func (c *Client) TestTrafficExportSink(ctx context.Context, id string) (TrafficExportTestResult, error) {
	var out TrafficExportTestResult
	err := c.do(ctx, http.MethodPost, trafficExportSinkPath(id)+"/test", nil, &out)
	return out, err
}

// GetTrafficExportStatus calls GET /api/v1/settings/traffic-export/status.
func (c *Client) GetTrafficExportStatus(ctx context.Context) (TrafficExportStatus, error) {
	var out TrafficExportStatus
	err := c.do(ctx, http.MethodGet, trafficExportBase+"/status", nil, &out)
	return out, err
}
