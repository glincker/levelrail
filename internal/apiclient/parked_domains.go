package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ParkedDomain mirrors internal/api's parkedDomainResource.
type ParkedDomain struct {
	Domain     string `json:"domain"`
	Kind       string `json:"kind"`
	ParkedFrom string `json:"parked_from,omitempty"`
	Note       string `json:"note,omitempty"`
	ParkedBy   string `json:"parked_by,omitempty"`
	ParkedAt   string `json:"parked_at,omitempty"`
}

// DomainLifecycleRequest is the body of park, move and attach.
type DomainLifecycleRequest struct {
	Domain  string `json:"domain,omitempty"`
	App     string `json:"app,omitempty"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Note    string `json:"note,omitempty"`
	Confirm bool   `json:"confirm,omitempty"`
}

// DomainLifecycleResult mirrors internal/api's domainLifecycleResponse.
type DomainLifecycleResult struct {
	Domain  string `json:"domain"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Applied bool   `json:"applied"`
}

// ListParkedDomains calls GET /api/v1/domains/parked.
func (c *Client) ListParkedDomains(ctx context.Context) ([]ParkedDomain, error) {
	var out []ParkedDomain
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/parked", nil, &out)
	return out, err
}

// ParkDomain calls POST /api/v1/domains/park.
func (c *Client) ParkDomain(ctx context.Context, req DomainLifecycleRequest) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/park", req, &out)
	return out, err
}

// MoveDomain calls POST /api/v1/domains/move.
func (c *Client) MoveDomain(ctx context.Context, req DomainLifecycleRequest) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/move", req, &out)
	return out, err
}

// AttachParkedDomain calls POST /api/v1/domains/parked/{domain}/attach.
func (c *Client) AttachParkedDomain(ctx context.Context, domain, app string) (DomainLifecycleResult, error) {
	var out DomainLifecycleResult
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/parked/"+url.PathEscape(domain)+"/attach", DomainLifecycleRequest{App: app}, &out)
	return out, err
}

// DeleteParkedDomain calls DELETE /api/v1/domains/parked/{domain}.
func (c *Client) DeleteParkedDomain(ctx context.Context, domain string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/domains/parked/"+url.PathEscape(domain), nil, nil)
}
