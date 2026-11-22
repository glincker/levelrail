package apiclient

import (
	"context"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

// DNSHostDetection is the response of GET /api/v1/domains/dns-provider and /api/v1/mail/dns/provider.
type DNSHostDetection struct {
	mailrecords.Detection
	Connected            bool   `json:"connected"`
	Connectable          bool   `json:"connectable"`
	ZoneID               string `json:"zone_id,omitempty"`
	DashboardURL         string `json:"dashboard_url,omitempty"`
	CanIssueCertificates bool   `json:"can_issue_certificates"`
	Hint                 string `json:"hint"`
}

// MailDNSDomain is one candidate of GET /api/v1/mail/dns/domains.
type MailDNSDomain struct {
	Domain   string   `json:"domain"`
	Source   string   `json:"source"`
	Sources  []string `json:"sources"`
	Provider string   `json:"provider,omitempty"`
}

// MailDNSDomains is the response of GET /api/v1/mail/dns/domains.
type MailDNSDomains struct {
	Domains  []MailDNSDomain `json:"domains"`
	Warnings []string        `json:"warnings,omitempty"`
}

// MailDNSAppliedEntry is one apply in GET /api/v1/mail/dns/applied.
type MailDNSAppliedEntry struct {
	AppliedAt string                      `json:"applied_at"`
	Actor     string                      `json:"actor"`
	Provider  string                      `json:"provider"`
	ZoneID    string                      `json:"zone_id"`
	Records   []mailrecords.AppliedRecord `json:"records"`
}

// MailDNSApplied is the response of GET /api/v1/mail/dns/applied.
type MailDNSApplied struct {
	Domain  string                `json:"domain"`
	Entries []MailDNSAppliedEntry `json:"entries"`
}

// DomainDNSProvider calls GET /api/v1/domains/dns-provider.
func (c *Client) DomainDNSProvider(ctx context.Context, domain string) (DNSHostDetection, error) {
	return getJSON[DNSHostDetection](ctx, c, "/api/v1/domains/dns-provider?"+url.Values{"domain": {domain}}.Encode())
}

// MailDNSProvider calls GET /api/v1/mail/dns/provider.
func (c *Client) MailDNSProvider(ctx context.Context, domain string) (DNSHostDetection, error) {
	return getJSON[DNSHostDetection](ctx, c, mailDNSPath("/provider", "", url.Values{"domain": {domain}}))
}

// MailDNSDomains calls GET /api/v1/mail/dns/domains.
func (c *Client) MailDNSDomains(ctx context.Context) (MailDNSDomains, error) {
	return getJSON[MailDNSDomains](ctx, c, mailDNSPath("/domains", "", nil))
}

// MailDNSApplied calls GET /api/v1/mail/dns/applied.
func (c *Client) MailDNSApplied(ctx context.Context, domain string) (MailDNSApplied, error) {
	return getJSON[MailDNSApplied](ctx, c, mailDNSPath("/applied", "", url.Values{"domain": {domain}}))
}
