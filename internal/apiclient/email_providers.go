package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// EmailProvider mirrors one entry of /api/v1/settings/email/providers. Credential fields are write-only: a response
// carries only the *_set booleans, and an empty credential on a write keeps the stored value.
type EmailProvider struct {
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Name    string `json:"name,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	Backend string `json:"backend"`

	TLSMode       string `json:"tls_mode,omitempty"`
	TLSSkipVerify bool   `json:"tls_skip_verify,omitempty"`

	SMTPHost        string `json:"smtp_host,omitempty"`
	SMTPPort        int    `json:"smtp_port,omitempty"`
	SMTPUsername    string `json:"smtp_username,omitempty"`
	SMTPFrom        string `json:"smtp_from,omitempty"`
	SMTPPassword    string `json:"smtp_password,omitempty"`
	SMTPPasswordSet bool   `json:"smtp_password_set,omitempty"`

	SESRegion             string `json:"ses_region,omitempty"`
	SESAccessKeyID        string `json:"ses_access_key_id,omitempty"`
	SESFrom               string `json:"ses_from,omitempty"`
	SESSecretAccessKey    string `json:"ses_secret_access_key,omitempty"`
	SESSecretAccessKeySet bool   `json:"ses_secret_access_key_set,omitempty"`

	SESSMTPPort        int    `json:"ses_smtp_port,omitempty"`
	SESSMTPUsername    string `json:"ses_smtp_username,omitempty"`
	SESSMTPPassword    string `json:"ses_smtp_password,omitempty"`
	SESSMTPPasswordSet bool   `json:"ses_smtp_password_set,omitempty"`

	ResendFrom      string `json:"resend_from,omitempty"`
	ResendAPIKey    string `json:"resend_api_key,omitempty"`
	ResendAPIKeySet bool   `json:"resend_api_key_set,omitempty"`

	PostmarkServerToken    string `json:"postmark_server_token,omitempty"`
	PostmarkServerTokenSet bool   `json:"postmark_server_token_set,omitempty"`
	SendGridAPIKey         string `json:"sendgrid_api_key,omitempty"`
	SendGridAPIKeySet      bool   `json:"sendgrid_api_key_set,omitempty"`
	MailgunAPIKey          string `json:"mailgun_api_key,omitempty"`
	MailgunAPIKeySet       bool   `json:"mailgun_api_key_set,omitempty"`
	MailgunDomain          string `json:"mailgun_domain,omitempty"`
	MailgunRegion          string `json:"mailgun_region,omitempty"`

	LastOKAt      *time.Time `json:"last_ok_at,omitempty"`
	LastErrorKind string     `json:"last_error_kind,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// SecretSet reports whether the provider's own backend has a stored credential.
func (p EmailProvider) SecretSet() bool {
	switch p.Backend {
	case EmailBackendSMTP:
		return p.SMTPPasswordSet
	case EmailBackendSES:
		return p.SESSecretAccessKeySet
	case "ses_smtp":
		return p.SESSMTPPasswordSet
	case "resend":
		return p.ResendAPIKeySet
	case "postmark":
		return p.PostmarkServerTokenSet
	case "sendgrid":
		return p.SendGridAPIKeySet
	case "mailgun":
		return p.MailgunAPIKeySet
	}
	return false
}

// EmailProviderList mirrors GET /api/v1/settings/email/providers.
type EmailProviderList struct {
	Providers []EmailProvider `json:"providers"`
}

func emailProviderPath(id string) string {
	return "/api/v1/settings/email/providers/" + url.PathEscape(id)
}

// ListEmailProviders calls GET /api/v1/settings/email/providers: the primary first, then the backup if any.
func (c *Client) ListEmailProviders(ctx context.Context) (EmailProviderList, error) {
	var out EmailProviderList
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/email/providers", nil, &out)
	return out, err
}

// GetEmailProvider calls GET /api/v1/settings/email/providers/{id}.
func (c *Client) GetEmailProvider(ctx context.Context, id string) (EmailProvider, error) {
	var out EmailProvider
	err := c.do(ctx, http.MethodGet, emailProviderPath(id), nil, &out)
	return out, err
}

// CreateEmailProvider calls POST /api/v1/settings/email/providers.
func (c *Client) CreateEmailProvider(ctx context.Context, req EmailProvider) (EmailProvider, error) {
	var out EmailProvider
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/email/providers", req, &out)
	return out, err
}

// UpdateEmailProvider calls PUT /api/v1/settings/email/providers/{id}, which replaces the provider's fields.
func (c *Client) UpdateEmailProvider(ctx context.Context, id string, req EmailProvider) (EmailProvider, error) {
	var out EmailProvider
	err := c.do(ctx, http.MethodPut, emailProviderPath(id), req, &out)
	return out, err
}

// DeleteEmailProvider calls DELETE /api/v1/settings/email/providers/{id}.
func (c *Client) DeleteEmailProvider(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, emailProviderPath(id), nil, nil)
}

// TestEmailProvider calls POST /api/v1/settings/email/providers/{id}/test: one send through that provider alone.
func (c *Client) TestEmailProvider(ctx context.Context, id, to string) (EmailTestResult, error) {
	var out EmailTestResult
	err := c.do(ctx, http.MethodPost, emailProviderPath(id)+"/test", map[string]string{"to": to}, &out)
	return out, err
}

// PromoteEmailProvider calls POST /api/v1/settings/email/providers/{id}/promote and returns the providers after the swap.
func (c *Client) PromoteEmailProvider(ctx context.Context, id string) (EmailProviderList, error) {
	var out EmailProviderList
	err := c.do(ctx, http.MethodPost, emailProviderPath(id)+"/promote", nil, &out)
	return out, err
}
