package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// EmailFooterLink is one labelled https link in the email footer.
type EmailFooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// EmailBranding is the branding stored on the email_settings identity row. Empty fields fall back to the brand layer at render time.
type EmailBranding struct {
	LogoURL       string            `json:"logo_url"`
	BrandColor    string            `json:"brand_color"`
	FooterText    string            `json:"footer_text"`
	FooterAddress string            `json:"footer_address"`
	FooterLinks   []EmailFooterLink `json:"footer_links"`
	ShowPoweredBy bool              `json:"show_powered_by"`
}

// GetEmailBranding returns the branding columns of the single email_settings row.
func (db *DB) GetEmailBranding(ctx context.Context) (EmailBranding, error) {
	var b EmailBranding
	var links string
	var powered int
	err := db.QueryRowContext(ctx, `SELECT logo_url, brand_color, footer_text, footer_address, footer_links, show_powered_by FROM email_settings WHERE id = 1`).
		Scan(&b.LogoURL, &b.BrandColor, &b.FooterText, &b.FooterAddress, &links, &powered)
	if err != nil {
		return EmailBranding{}, fmt.Errorf("store: get email branding: %w", err)
	}
	if err := json.Unmarshal([]byte(links), &b.FooterLinks); err != nil {
		return EmailBranding{}, fmt.Errorf("store: decode email footer links: %w", err)
	}
	if b.FooterLinks == nil {
		b.FooterLinks = []EmailFooterLink{}
	}
	b.ShowPoweredBy = powered != 0
	return b, nil
}

// SetEmailBranding replaces only the branding columns, leaving provider settings alone.
func (db *DB) SetEmailBranding(ctx context.Context, b EmailBranding) error {
	if b.FooterLinks == nil {
		b.FooterLinks = []EmailFooterLink{}
	}
	links, err := json.Marshal(b.FooterLinks)
	if err != nil {
		return fmt.Errorf("store: encode email footer links: %w", err)
	}
	powered := 0
	if b.ShowPoweredBy {
		powered = 1
	}
	_, err = db.ExecContext(ctx, `UPDATE email_settings SET logo_url = ?, brand_color = ?, footer_text = ?, footer_address = ?, footer_links = ?, show_powered_by = ? WHERE id = 1`,
		b.LogoURL, b.BrandColor, b.FooterText, b.FooterAddress, string(links), powered)
	if err != nil {
		return fmt.Errorf("store: set email branding: %w", err)
	}
	return nil
}
