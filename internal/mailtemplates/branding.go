package mailtemplates

import (
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// FooterLink is one labelled https link in the email footer.
type FooterLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// EmailBranding is what an operator controls about how every rich email looks.
// The footer is structured text plus allowlisted links: there is deliberately no raw HTML field.
type EmailBranding struct {
	LogoURL       string       `json:"logo_url"`
	BrandColor    string       `json:"brand_color"`
	FooterText    string       `json:"footer_text"`
	FooterAddress string       `json:"footer_address"`
	FooterLinks   []FooterLink `json:"footer_links"`
	ShowPoweredBy bool         `json:"show_powered_by"`
}

// BrandingLimits caps the size of each branding field.
type BrandingLimits struct {
	MaxLogoURL       int
	MaxFooterText    int
	MaxFooterAddress int
	MaxFooterLinks   int
	MaxLinkLabel     int
}

// Env names for the branding caps; each falls back to the default below when unset or invalid.
const (
	EnvMaxLogoURL       = "APP_EMAIL_BRANDING_MAX_LOGO_URL"
	EnvMaxFooterText    = "APP_EMAIL_BRANDING_MAX_FOOTER_TEXT"
	EnvMaxFooterAddress = "APP_EMAIL_BRANDING_MAX_FOOTER_ADDRESS"
	EnvMaxFooterLinks   = "APP_EMAIL_BRANDING_MAX_FOOTER_LINKS"
	EnvMaxLinkLabel     = "APP_EMAIL_BRANDING_MAX_LINK_LABEL"
)

// LimitsFromEnv reads the branding caps from the environment.
func LimitsFromEnv() BrandingLimits {
	return BrandingLimits{
		MaxLogoURL:       envInt(EnvMaxLogoURL, 500),
		MaxFooterText:    envInt(EnvMaxFooterText, 1000),
		MaxFooterAddress: envInt(EnvMaxFooterAddress, 300),
		MaxFooterLinks:   envInt(EnvMaxFooterLinks, 5),
		MaxLinkLabel:     envInt(EnvMaxLinkLabel, 60),
	}
}

func envInt(name string, def int) int {
	if v, ok := os.LookupEnv(name); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return def
}

var brandColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidateHTTPSURL accepts an empty string or an https URL with a host, no credentials and no whitespace or control characters.
// The server never fetches it; it only lands in an href or img src.
func ValidateHTTPSURL(raw string, maxLen int) error {
	if raw == "" {
		return nil
	}
	if len(raw) > maxLen {
		return fmt.Errorf("must be at most %d characters", maxLen)
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return errors.New("must not contain spaces or control characters")
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("must be an https URL")
	}
	return nil
}

func validateFooter(text, address string, links []FooterLink, lim BrandingLimits) error {
	if len(text) > lim.MaxFooterText {
		return fmt.Errorf("footer text must be at most %d characters", lim.MaxFooterText)
	}
	if len(address) > lim.MaxFooterAddress {
		return fmt.Errorf("footer address must be at most %d characters", lim.MaxFooterAddress)
	}
	if len(links) > lim.MaxFooterLinks {
		return fmt.Errorf("at most %d footer links are allowed", lim.MaxFooterLinks)
	}
	for _, f := range []struct{ name, v string }{{"footer text", text}, {"footer address", address}} {
		if err := ValidateTokens(f.v, tokensByScope(ScopeGlobal)); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	for i, l := range links {
		if strings.TrimSpace(l.Label) == "" || len(l.Label) > lim.MaxLinkLabel || strings.ContainsAny(l.Label, "\r\n") {
			return fmt.Errorf("footer link %d needs a single-line label of at most %d characters", i+1, lim.MaxLinkLabel)
		}
		if l.URL == "" {
			return fmt.Errorf("footer link %d needs a URL", i+1)
		}
		if err := ValidateHTTPSURL(l.URL, lim.MaxLogoURL); err != nil {
			return fmt.Errorf("footer link %d URL %w", i+1, err)
		}
	}
	return nil
}

// ValidateBranding refuses anything unsafe or oversized; it is the single save-time gate.
func ValidateBranding(b EmailBranding, lim BrandingLimits) error {
	if err := ValidateHTTPSURL(b.LogoURL, lim.MaxLogoURL); err != nil {
		return fmt.Errorf("logo URL %w", err)
	}
	if b.BrandColor != "" && !brandColorRe.MatchString(b.BrandColor) {
		return errors.New("brand color must be a hex color such as #107292")
	}
	return validateFooter(b.FooterText, b.FooterAddress, b.FooterLinks, lim)
}

// Layout turns validated branding into the layout the renderer wears.
func (b EmailBranding) Layout() Layout {
	return Layout{HeaderImageURL: b.LogoURL, FooterText: b.FooterText, FooterAddress: b.FooterAddress, FooterLinks: b.FooterLinks, ShowPoweredBy: b.ShowPoweredBy}
}

func footerExtrasHTML(l Layout, b Branding, vals map[string]string) string {
	var parts []string
	if a := footerHTML(l.FooterAddress, vals); a != "" {
		parts = append(parts, a)
	}
	var links []string
	for _, fl := range l.FooterLinks {
		if ValidateHTTPSURL(fl.URL, 2048) != nil || fl.URL == "" {
			continue
		}
		links = append(links, `<a class="lnk" href="`+html.EscapeString(fl.URL)+`" style="color:#6b7280;text-decoration:underline">`+html.EscapeString(flatten(fl.Label))+`</a>`)
	}
	if len(links) > 0 {
		parts = append(parts, strings.Join(links, " &middot; "))
	}
	if l.ShowPoweredBy && b.Name != "" {
		parts = append(parts, "Powered by "+html.EscapeString(b.Name))
	}
	return strings.Join(parts, "<br>")
}

func footerExtrasText(l Layout, b Branding, vals map[string]string) string {
	var lines []string
	if a := strings.TrimSpace(SubstituteText(strings.ReplaceAll(l.FooterAddress, "\r", ""), vals)); a != "" {
		lines = append(lines, a)
	}
	for _, fl := range l.FooterLinks {
		if ValidateHTTPSURL(fl.URL, 2048) != nil || fl.URL == "" {
			continue
		}
		lines = append(lines, flatten(fl.Label)+": "+fl.URL)
	}
	if l.ShowPoweredBy && b.Name != "" {
		lines = append(lines, "Powered by "+flatten(b.Name))
	}
	return strings.Join(lines, "\n")
}

func flatten(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
