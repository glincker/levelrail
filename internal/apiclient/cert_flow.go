package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// CertPreflightItem is one preflight finding for a public certificate request.
type CertPreflightItem struct {
	Code   string `json:"code"`
	State  string `json:"state"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// CertPreflight mirrors GET /api/v1/domains/{domain}/cert/preflight.
type CertPreflight struct {
	Domain     string              `json:"domain"`
	Staging    bool                `json:"staging"`
	Email      string              `json:"email"`
	Ready      bool                `json:"ready"`
	Items      []CertPreflightItem `json:"items"`
	NextSafeAt *time.Time          `json:"next_safe_at,omitempty"`
}

// CertRenewal is the last issue or renewal attempt.
type CertRenewal struct {
	At     time.Time `json:"at"`
	Result string    `json:"result"`
	Error  string    `json:"error,omitempty"`
}

// DomainCertStatus mirrors GET /api/v1/domains/{domain}/cert/status: state is
// off, pending, issued or failed.
type DomainCertStatus struct {
	Domain       string       `json:"domain"`
	State        string       `json:"state"`
	Staging      bool         `json:"staging"`
	AppliesToAll bool         `json:"applies_to_all"`
	Issuer       string       `json:"issuer,omitempty"`
	Kind         string       `json:"kind"`
	NotAfter     *time.Time   `json:"not_after,omitempty"`
	DaysLeft     *int         `json:"days_left,omitempty"`
	RenewalAt    *time.Time   `json:"renewal_at,omitempty"`
	LastRenewal  *CertRenewal `json:"last_renewal,omitempty"`
	Error        string       `json:"error,omitempty"`
	Explanation  string       `json:"explanation,omitempty"`
	Hint         string       `json:"hint,omitempty"`
	NextSafeAt   *time.Time   `json:"next_safe_at,omitempty"`
}

// GetCertRequest is POST /api/v1/domains/{domain}/cert's body.
type GetCertRequest struct {
	Email       string   `json:"email,omitempty"`
	Staging     bool     `json:"staging"`
	Acknowledge []string `json:"acknowledge,omitempty"`
}

// CertPreflight runs the preflight for a public certificate.
func (c *Client) CertPreflight(ctx context.Context, domain, email string, staging bool) (CertPreflight, error) {
	q := url.Values{}
	if email != "" {
		q.Set("email", email)
	}
	if staging {
		q.Set("staging", "true")
	}
	path := "/api/v1/domains/" + url.PathEscape(domain) + "/cert/preflight"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out CertPreflight
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetCertificate asks for a real certificate for domain.
func (c *Client) GetCertificate(ctx context.Context, domain string, req GetCertRequest) (DomainCertStatus, error) {
	var out DomainCertStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/domains/"+url.PathEscape(domain)+"/cert", req, &out)
	return out, err
}

// DomainCertStatus reads the progress and details of domain's certificate.
func (c *Client) DomainCertStatus(ctx context.Context, domain string) (DomainCertStatus, error) {
	var out DomainCertStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/"+url.PathEscape(domain)+"/cert/status", nil, &out)
	return out, err
}
