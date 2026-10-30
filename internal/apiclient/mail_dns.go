package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

// MailDNSRequest is the body of the mail DNS plan and apply calls. Confirm and PlanHash are used by apply only.
type MailDNSRequest struct {
	mailrecords.Spec
	ReplaceExisting []string `json:"replace_existing,omitempty"`
	Confirm         bool     `json:"confirm,omitempty"`
	PlanHash        string   `json:"plan_hash,omitempty"`
}

// MailDNSApplyResult is the response of the apply call.
type MailDNSApplyResult struct {
	Applied int                    `json:"applied"`
	Plan    mailrecords.PlanResult `json:"plan"`
}

func mailDNSPath(path, provider string, q url.Values) string {
	if q == nil {
		q = url.Values{}
	}
	if provider != "" {
		q.Set("provider", provider)
	}
	p := "/api/v1/mail/dns" + path
	if enc := q.Encode(); enc != "" {
		p += "?" + enc
	}
	return p
}

// PlanMailDNS calls POST /api/v1/mail/dns/plan.
func (c *Client) PlanMailDNS(ctx context.Context, provider string, req MailDNSRequest) (mailrecords.PlanResult, error) {
	var out mailrecords.PlanResult
	err := c.do(ctx, http.MethodPost, mailDNSPath("/plan", provider, nil), req, &out)
	return out, err
}

// ApplyMailDNS calls POST /api/v1/mail/dns/apply.
func (c *Client) ApplyMailDNS(ctx context.Context, provider string, req MailDNSRequest) (MailDNSApplyResult, error) {
	var out MailDNSApplyResult
	err := c.do(ctx, http.MethodPost, mailDNSPath("/apply", provider, nil), req, &out)
	return out, err
}

// CheckMailDNS calls GET /api/v1/mail/dns/check.
func (c *Client) CheckMailDNS(ctx context.Context, domain, mailHost, selector string) (mailrecords.Report, error) {
	q := url.Values{"domain": {domain}}
	if mailHost != "" {
		q.Set("mail_host", mailHost)
	}
	if selector != "" {
		q.Set("dkim_selector", selector)
	}
	var out mailrecords.Report
	err := c.do(ctx, http.MethodGet, mailDNSPath("/check", "", q), nil, &out)
	return out, err
}

// MailStatus is the response of GET /api/v1/mail/status.
type MailStatus struct {
	App            string `json:"app"`
	Installed      bool   `json:"installed"`
	ContainerState string `json:"container_state"`
	AdminReachable bool   `json:"admin_reachable"`
	TLS            string `json:"tls"`
	NextStep       string `json:"next_step"`
	Domain         string `json:"domain,omitempty"`
	Hostname       string `json:"hostname,omitempty"`
	// MaintenanceHold is set while a restore holds the mail app stopped.
	MaintenanceHold string `json:"maintenance_hold,omitempty"`
}

// MailStatus calls GET /api/v1/mail/status; app picks one install when several exist.
func (c *Client) MailStatus(ctx context.Context, app string) (MailStatus, error) {
	var out MailStatus
	path := "/api/v1/mail/status"
	if app != "" {
		path += "?app=" + url.QueryEscape(app)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
