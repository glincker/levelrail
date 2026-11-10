package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DomainTuningResource mirrors internal/api's domainTuningResource.
type DomainTuningResource struct {
	Domain                       string     `json:"domain"`
	Configured                   bool       `json:"configured"`
	Effective                    bool       `json:"effective"`
	Preset                       string     `json:"preset"`
	Compression                  bool       `json:"compression"`
	Streaming                    bool       `json:"streaming"`
	WebSocket                    bool       `json:"websocket"`
	DialTimeoutSeconds           int        `json:"dial_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int        `json:"response_header_timeout_seconds"`
	StreamTimeoutSeconds         int        `json:"stream_timeout_seconds"`
	StreamCloseDelaySeconds      int        `json:"stream_close_delay_seconds"`
	MinTimeoutSeconds            int        `json:"min_timeout_seconds"`
	MaxTimeoutSeconds            int        `json:"max_timeout_seconds"`
	MaxStreamSeconds             int        `json:"max_stream_seconds"`
	UpdatedAt                    *time.Time `json:"updated_at,omitempty"`
}

// SetDomainTuningRequest is the PUT body; 0 or false keeps the ingress default.
type SetDomainTuningRequest struct {
	Preset                       string `json:"preset"`
	Compression                  bool   `json:"compression"`
	Streaming                    bool   `json:"streaming"`
	WebSocket                    bool   `json:"websocket"`
	DialTimeoutSeconds           int    `json:"dial_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int    `json:"response_header_timeout_seconds"`
	StreamTimeoutSeconds         int    `json:"stream_timeout_seconds"`
	StreamCloseDelaySeconds      int    `json:"stream_close_delay_seconds"`
}

func domainTuningPath(name, domain string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/domains/" + PathEscape(domain) + "/tuning"
}

// GetDomainTuning calls GET .../domains/{domain}/tuning.
func (c *Client) GetDomainTuning(ctx context.Context, name, domain string) (DomainTuningResource, error) {
	var out DomainTuningResource
	err := c.do(ctx, http.MethodGet, domainTuningPath(name, domain), nil, &out)
	return out, err
}

// SetDomainTuning calls PUT .../domains/{domain}/tuning.
func (c *Client) SetDomainTuning(ctx context.Context, name, domain string, req SetDomainTuningRequest) (DomainTuningResource, error) {
	var out DomainTuningResource
	err := c.do(ctx, http.MethodPut, domainTuningPath(name, domain), req, &out)
	return out, err
}

// ClearDomainTuning calls DELETE .../domains/{domain}/tuning.
func (c *Client) ClearDomainTuning(ctx context.Context, name, domain string) (DomainTuningResource, error) {
	var out DomainTuningResource
	err := c.do(ctx, http.MethodDelete, domainTuningPath(name, domain), nil, &out)
	return out, err
}
