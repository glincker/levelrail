package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DomainLimitsResource mirrors internal/api's domainLimitsResource.
type DomainLimitsResource struct {
	Domain                string     `json:"domain"`
	Configured            bool       `json:"configured"`
	MaxBodyBytes          int64      `json:"max_body_bytes"`
	ReadTimeoutSeconds    int        `json:"read_timeout_seconds"`
	WriteTimeoutSeconds   int        `json:"write_timeout_seconds"`
	MinBodyBytes          int64      `json:"min_body_bytes"`
	CeilingBodyBytes      int64      `json:"ceiling_body_bytes"`
	CeilingTimeoutSeconds int        `json:"ceiling_timeout_seconds"`
	UpdatedAt             *time.Time `json:"updated_at,omitempty"`
}

// SetDomainLimitsRequest is the PUT body; 0 means no per-domain value.
type SetDomainLimitsRequest struct {
	MaxBodyBytes        int64 `json:"max_body_bytes"`
	ReadTimeoutSeconds  int   `json:"read_timeout_seconds"`
	WriteTimeoutSeconds int   `json:"write_timeout_seconds"`
}

func domainLimitsPath(name, domain string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/domains/" + PathEscape(domain) + "/limits"
}

// GetDomainLimits calls GET .../domains/{domain}/limits.
func (c *Client) GetDomainLimits(ctx context.Context, name, domain string) (DomainLimitsResource, error) {
	var out DomainLimitsResource
	err := c.do(ctx, http.MethodGet, domainLimitsPath(name, domain), nil, &out)
	return out, err
}

// SetDomainLimits calls PUT .../domains/{domain}/limits.
func (c *Client) SetDomainLimits(ctx context.Context, name, domain string, req SetDomainLimitsRequest) (DomainLimitsResource, error) {
	var out DomainLimitsResource
	err := c.do(ctx, http.MethodPut, domainLimitsPath(name, domain), req, &out)
	return out, err
}

// ClearDomainLimits calls DELETE .../domains/{domain}/limits.
func (c *Client) ClearDomainLimits(ctx context.Context, name, domain string) (DomainLimitsResource, error) {
	var out DomainLimitsResource
	err := c.do(ctx, http.MethodDelete, domainLimitsPath(name, domain), nil, &out)
	return out, err
}
