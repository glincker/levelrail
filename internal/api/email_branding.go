package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

// emailBrandingResource is the wire shape for the branding endpoints. The server never fetches logo_url,
// it only lands in an img src, so a remote logo tells its host when a message is opened.
type emailBrandingResource struct {
	LogoURL       string                     `json:"logo_url"`
	BrandColor    string                     `json:"brand_color"`
	FooterText    string                     `json:"footer_text"`
	FooterAddress string                     `json:"footer_address"`
	FooterLinks   []mailtemplates.FooterLink `json:"footer_links"`
	ShowPoweredBy bool                       `json:"show_powered_by"`

	// Read-only context for editors.
	DefaultBrandColor string   `json:"default_brand_color,omitempty"`
	MaxFooterLinks    int      `json:"max_footer_links,omitempty"`
	Variables         []string `json:"variables,omitempty"`
}

type emailBrandingPreviewResponse struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

// emailBranding loads the stored branding; a read failure logs and falls back to none so mail still sends.
func (rt *Router) emailBranding(ctx context.Context) mailtemplates.EmailBranding {
	if rt.emailSettings == nil {
		return mailtemplates.EmailBranding{}
	}
	b, err := rt.emailSettings.GetEmailBranding(ctx)
	if err != nil {
		rt.logger.Warn("api: load email branding failed", slog.String("error", err.Error()))
		return mailtemplates.EmailBranding{}
	}
	return brandingFromStore(b)
}

func brandingFromStore(b store.EmailBranding) mailtemplates.EmailBranding {
	links := make([]mailtemplates.FooterLink, 0, len(b.FooterLinks))
	for _, l := range b.FooterLinks {
		links = append(links, mailtemplates.FooterLink{Label: l.Label, URL: l.URL})
	}
	return mailtemplates.EmailBranding{
		LogoURL: b.LogoURL, BrandColor: b.BrandColor, FooterText: b.FooterText,
		FooterAddress: b.FooterAddress, FooterLinks: links, ShowPoweredBy: b.ShowPoweredBy,
	}
}

func brandingToStore(b mailtemplates.EmailBranding) store.EmailBranding {
	links := make([]store.EmailFooterLink, 0, len(b.FooterLinks))
	for _, l := range b.FooterLinks {
		links = append(links, store.EmailFooterLink{Label: l.Label, URL: l.URL})
	}
	return store.EmailBranding{
		LogoURL: b.LogoURL, BrandColor: b.BrandColor, FooterText: b.FooterText,
		FooterAddress: b.FooterAddress, FooterLinks: links, ShowPoweredBy: b.ShowPoweredBy,
	}
}

func (rt *Router) brandingResource(b mailtemplates.EmailBranding) emailBrandingResource {
	if b.FooterLinks == nil {
		b.FooterLinks = []mailtemplates.FooterLink{}
	}
	var vars []string
	for _, t := range mailtemplates.Tokens() {
		if t.Scope == mailtemplates.ScopeGlobal {
			vars = append(vars, t.Name)
		}
	}
	return emailBrandingResource{
		LogoURL: b.LogoURL, BrandColor: b.BrandColor, FooterText: b.FooterText, FooterAddress: b.FooterAddress,
		FooterLinks: b.FooterLinks, ShowPoweredBy: b.ShowPoweredBy,
		DefaultBrandColor: rt.brand.PrimaryColor, MaxFooterLinks: mailtemplates.LimitsFromEnv().MaxFooterLinks, Variables: vars,
	}
}

// normalize trims single-line fields and CRLF-normalizes the multi-line ones.
func (req emailBrandingResource) normalize() mailtemplates.EmailBranding {
	b := mailtemplates.EmailBranding{
		LogoURL: strings.TrimSpace(req.LogoURL), BrandColor: strings.TrimSpace(req.BrandColor),
		FooterText: normalizeNewlines(req.FooterText), FooterAddress: normalizeNewlines(req.FooterAddress),
		ShowPoweredBy: req.ShowPoweredBy,
	}
	for _, l := range req.FooterLinks {
		b.FooterLinks = append(b.FooterLinks, mailtemplates.FooterLink{Label: strings.TrimSpace(l.Label), URL: strings.TrimSpace(l.URL)})
	}
	return b
}

func normalizeNewlines(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
}

// handleGetEmailBranding handles GET /api/v1/settings/email/branding.
func (rt *Router) handleGetEmailBranding(w http.ResponseWriter, r *http.Request) {
	b, err := rt.emailSettings.GetEmailBranding(r.Context())
	if err != nil {
		rt.internalError(w, "api: get email branding failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.brandingResource(brandingFromStore(b)))
}

// handleUpdateEmailBranding handles PUT /api/v1/settings/email/branding.
func (rt *Router) handleUpdateEmailBranding(w http.ResponseWriter, r *http.Request) {
	var req emailBrandingResource
	if !decodeReq(w, r, &req) {
		return
	}
	b := req.normalize()
	if err := mailtemplates.ValidateBranding(b, mailtemplates.LimitsFromEnv()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.emailSettings.SetEmailBranding(r.Context(), brandingToStore(b)); err != nil {
		rt.internalError(w, "api: update email branding failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.brandingResource(b))
}

// handlePreviewEmailBranding handles POST /api/v1/settings/email/branding/preview.
// It renders the posted, unsaved branding around a sample message and never sends or stores anything.
func (rt *Router) handlePreviewEmailBranding(w http.ResponseWriter, r *http.Request) {
	var req emailBrandingResource
	if !decodeReq(w, r, &req) {
		return
	}
	b := req.normalize()
	if err := mailtemplates.ValidateBranding(b, mailtemplates.LimitsFromEnv()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ev, ok := mailtemplates.LookupEvent(mailtemplates.EventPasswordReset)
	if !ok {
		rt.internalError(w, "api: email branding preview: sample template missing", nil)
		return
	}
	branding := rt.mailBranding(r.Context())
	branding.PrimaryColor, branding.PrimaryColorDark = rt.brand.PrimaryColor, rt.brand.PrimaryColorDark
	branding = withBrandColor(branding, b.BrandColor)
	out := mailtemplates.Render(branding, overlayBranding(mailtemplates.Layout{}, b), ev.Default, mailtemplates.SampleValues(), mailtemplates.Options{})
	writeJSON(w, http.StatusOK, emailBrandingPreviewResponse{Subject: out.Subject, HTML: out.HTML, Text: out.Text})
}
