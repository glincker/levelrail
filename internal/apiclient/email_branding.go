package apiclient

import (
	"context"
	"net/http"
)

// EmailFooterLink is one labelled https link in the email footer.
type EmailFooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// EmailBranding mirrors GET and PUT /api/v1/settings/email/branding.
type EmailBranding struct {
	LogoURL       string            `json:"logo_url"`
	BrandColor    string            `json:"brand_color"`
	FooterText    string            `json:"footer_text"`
	FooterAddress string            `json:"footer_address"`
	FooterLinks   []EmailFooterLink `json:"footer_links"`
	ShowPoweredBy bool              `json:"show_powered_by"`

	DefaultBrandColor string   `json:"default_brand_color,omitempty"`
	MaxFooterLinks    int      `json:"max_footer_links,omitempty"`
	Variables         []string `json:"variables,omitempty"`
}

// EmailBrandingPreview mirrors POST /api/v1/settings/email/branding/preview.
type EmailBrandingPreview struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

// GetEmailBranding calls GET /api/v1/settings/email/branding.
func (c *Client) GetEmailBranding(ctx context.Context) (EmailBranding, error) {
	var out EmailBranding
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/email/branding", nil, &out)
	return out, err
}

// UpdateEmailBranding calls PUT /api/v1/settings/email/branding, replacing the whole branding.
func (c *Client) UpdateEmailBranding(ctx context.Context, req EmailBranding) (EmailBranding, error) {
	var out EmailBranding
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/email/branding", req, &out)
	return out, err
}

// PreviewEmailBranding calls POST /api/v1/settings/email/branding/preview with unsaved branding.
func (c *Client) PreviewEmailBranding(ctx context.Context, req EmailBranding) (EmailBrandingPreview, error) {
	var out EmailBrandingPreview
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/email/branding/preview", req, &out)
	return out, err
}
