package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// EmailTestResult mirrors POST /api/v1/settings/email/test.
type EmailTestResult struct {
	To        string `json:"to"`
	Provider  string `json:"provider"`
	MessageID string `json:"message_id,omitempty"`
	Attempts  int    `json:"attempts"`
}

// EmailStatus mirrors GET /api/v1/settings/email/status.
type EmailStatus struct {
	Configured    bool       `json:"configured"`
	Provider      string     `json:"provider"`
	From          string     `json:"from,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	LastErrorKind string     `json:"last_error_kind,omitempty"`
	Sent24h       int        `json:"sent_24h"`
	Failed24h     int        `json:"failed_24h"`
}

// EmailRecordCheck is one SPF, DKIM or DMARC verdict.
type EmailRecordCheck struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Code    string `json:"code"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
}

// EmailDomainCheck mirrors GET /api/v1/settings/email/domain-check.
type EmailDomainCheck struct {
	Domain     string             `json:"domain"`
	Provider   string             `json:"provider"`
	Records    []EmailRecordCheck `json:"records"`
	SPFInclude string             `json:"spf_include,omitempty"`
}

// EmailDelivery is one delivery log row.
type EmailDelivery struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Provider  string    `json:"provider"`
	To        string    `json:"to"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	MessageID string    `json:"message_id"`
	Error     string    `json:"error"`
	ErrorKind string    `json:"error_kind"`
	Attempts  int       `json:"attempts"`
}

// EmailDeliveryList mirrors GET /api/v1/settings/email/deliveries.
type EmailDeliveryList struct {
	Deliveries []EmailDelivery `json:"deliveries"`
	NextBefore string          `json:"next_before,omitempty"`
}

// EmailDeliveryQuery filters ListEmailDeliveries.
type EmailDeliveryQuery struct {
	Status, Provider, Kind, Query string
	Limit                         int
}

// SendTestEmail calls POST /api/v1/settings/email/test; an empty to means the signed-in user's own address.
func (c *Client) SendTestEmail(ctx context.Context, to string) (EmailTestResult, error) {
	var out EmailTestResult
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/email/test", map[string]string{"to": to}, &out)
	return out, err
}

// GetEmailStatus calls GET /api/v1/settings/email/status.
func (c *Client) GetEmailStatus(ctx context.Context) (EmailStatus, error) {
	var out EmailStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/email/status", nil, &out)
	return out, err
}

// CheckEmailDomain calls GET /api/v1/settings/email/domain-check; an empty domain uses the From address's domain.
func (c *Client) CheckEmailDomain(ctx context.Context, domain string) (EmailDomainCheck, error) {
	var out EmailDomainCheck
	path := "/api/v1/settings/email/domain-check"
	if domain != "" {
		path += "?domain=" + url.QueryEscape(domain)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// ListEmailDeliveries calls GET /api/v1/settings/email/deliveries.
func (c *Client) ListEmailDeliveries(ctx context.Context, q EmailDeliveryQuery) (EmailDeliveryList, error) {
	v := url.Values{}
	for k, val := range map[string]string{"status": q.Status, "provider": q.Provider, "kind": q.Kind, "q": q.Query} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	path := "/api/v1/settings/email/deliveries"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}
	var out EmailDeliveryList
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
