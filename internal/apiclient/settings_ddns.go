package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DDNSResource mirrors GET /api/v1/settings/ddns. The update URL or token is never returned, only its host.
type DDNSResource struct {
	Enabled           bool     `json:"enabled"`
	Provider          string   `json:"provider"`
	Domains           string   `json:"domains,omitempty"`
	IntervalSecs      int      `json:"interval_seconds"`
	ProbeURLs         []string `json:"probe_urls"`
	ProbeQuorum       int      `json:"probe_quorum"`
	TimeoutSecs       int      `json:"timeout_seconds"`
	MinUpdateGapSecs  int      `json:"min_update_gap_seconds"`
	ReassertSecs      int      `json:"reassert_seconds"`
	FailAlertAfter    int      `json:"fail_alert_after"`
	AllowPrivate      bool     `json:"allow_private"`
	AllowInsecureHTTP bool     `json:"allow_insecure_http"`
	Source            string   `json:"source"`
	TargetConfigured  bool     `json:"target_configured"`
	Host              string   `json:"host,omitempty"`
	Running           bool     `json:"running"`
}

// DDNSUpdate is the PUT body; nil fields keep the stored value and an empty target clears it.
type DDNSUpdate struct {
	Enabled           *bool     `json:"enabled,omitempty"`
	Provider          *string   `json:"provider,omitempty"`
	Domains           *string   `json:"domains,omitempty"`
	Target            *string   `json:"target,omitempty"`
	IntervalSecs      *int      `json:"interval_seconds,omitempty"`
	ProbeURLs         *[]string `json:"probe_urls,omitempty"`
	ProbeQuorum       *int      `json:"probe_quorum,omitempty"`
	TimeoutSecs       *int      `json:"timeout_seconds,omitempty"`
	MinUpdateGapSecs  *int      `json:"min_update_gap_seconds,omitempty"`
	ReassertSecs      *int      `json:"reassert_seconds,omitempty"`
	FailAlertAfter    *int      `json:"fail_alert_after,omitempty"`
	AllowPrivate      *bool     `json:"allow_private,omitempty"`
	AllowInsecureHTTP *bool     `json:"allow_insecure_http,omitempty"`
}

// DDNSStatus mirrors GET /api/v1/settings/ddns/status.
type DDNSStatus struct {
	Running     bool      `json:"running"`
	LastCheck   time.Time `json:"last_check,omitzero"`
	LastChange  time.Time `json:"last_change,omitzero"`
	LastIP      string    `json:"last_ip,omitempty"`
	LastResult  string    `json:"last_result"`
	LastError   string    `json:"last_error,omitempty"`
	Failures    int       `json:"consecutive_failures"`
	AlertActive bool      `json:"alert_active"`
}

// GetDDNS calls GET /api/v1/settings/ddns.
func (c *Client) GetDDNS(ctx context.Context) (DDNSResource, error) {
	var out DDNSResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/ddns", nil, &out)
	return out, err
}

// UpdateDDNS calls PUT /api/v1/settings/ddns.
func (c *Client) UpdateDDNS(ctx context.Context, req DDNSUpdate) (DDNSResource, error) {
	var out DDNSResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/ddns", req, &out)
	return out, err
}

// GetDDNSStatus calls GET /api/v1/settings/ddns/status.
func (c *Client) GetDDNSStatus(ctx context.Context) (DDNSStatus, error) {
	var out DDNSStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/ddns/status", nil, &out)
	return out, err
}
