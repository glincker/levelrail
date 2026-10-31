package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
)

// DNSStatusReport is the response of GET /api/v1/domains/dns-status and /api/v1/mail/dns/status.
type DNSStatusReport struct {
	Domain string `json:"domain"`
	App    string `json:"app,omitempty"`
	dnsstatus.Status
}

// DomainDNSStatus calls GET /api/v1/domains/dns-status.
func (c *Client) DomainDNSStatus(ctx context.Context, domain, app string) (DNSStatusReport, error) {
	q := url.Values{"domain": {domain}}
	if app != "" {
		q.Set("app", app)
	}
	var out DNSStatusReport
	err := c.do(ctx, http.MethodGet, "/api/v1/domains/dns-status?"+q.Encode(), nil, &out)
	return out, err
}

// MailDNSStatus calls GET /api/v1/mail/dns/status; q carries the spec fields beyond domain.
func (c *Client) MailDNSStatus(ctx context.Context, domain string, q url.Values) (DNSStatusReport, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("domain", domain)
	var out DNSStatusReport
	err := c.do(ctx, http.MethodGet, mailDNSPath("/status", "", q), nil, &out)
	return out, err
}
