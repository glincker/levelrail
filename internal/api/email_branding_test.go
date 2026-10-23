package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/mailtemplates"
)

const validBrandingBody = `{"logo_url":"https://cdn.example.com/logo.png","brand_color":"#112233","footer_text":"(c) {{year}} {{brand_name}}","footer_address":"1 Main St","footer_links":[{"label":"Privacy","url":"https://example.com/privacy"}],"show_powered_by":true}`

func TestEmailBranding_RoundTripAndPreview(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email/branding", validBrandingBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/email/branding", ""))
	var got emailBrandingResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.LogoURL != "https://cdn.example.com/logo.png" || got.BrandColor != "#112233" || len(got.FooterLinks) != 1 || !got.ShowPoweredBy {
		t.Errorf("GET = %+v", got)
	}
	if got.MaxFooterLinks == 0 || len(got.Variables) == 0 {
		t.Errorf("GET lacks editor context: %+v", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/email/branding/preview", validBrandingBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var prev emailBrandingPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	for _, want := range []string{`src="https://cdn.example.com/logo.png"`, "#112233", `href="https://example.com/privacy"`, "Powered by"} {
		if !strings.Contains(prev.HTML, want) {
			t.Errorf("preview html missing %q", want)
		}
	}
}

func TestEmailBranding_RejectsUnsafeInput(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	bodies := map[string]string{
		"javascript logo":  `{"logo_url":"javascript:alert(1)"}`,
		"data logo":        `{"logo_url":"data:image/png;base64,AAAA"}`,
		"http logo":        `{"logo_url":"http://example.com/x.png"}`,
		"oversized logo":   `{"logo_url":"https://example.com/` + strings.Repeat("a", 600) + `"}`,
		"bad color":        `{"brand_color":"blue"}`,
		"unknown variable": `{"footer_text":"{{not_a_variable}}"}`,
		"http footer link": `{"footer_links":[{"label":"x","url":"http://example.com"}]}`,
		"js footer link":   `{"footer_links":[{"label":"x","url":"javascript:alert(1)"}]}`,
	}
	for name, body := range bodies {
		for _, c := range []struct{ method, path string }{
			{http.MethodPut, "/api/v1/settings/email/branding"},
			{http.MethodPost, "/api/v1/settings/email/branding/preview"},
		} {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c.method, c.path, body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %s: status = %d, want 400", name, c.method, rec.Code)
			}
		}
	}
}

func TestEmailBranding_PreviewEscapesScript(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := `{"footer_address":"<script>alert(1)</script>","footer_links":[{"label":"<b>x</b>","url":"https://example.com"}]}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/email/branding/preview", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var prev emailBrandingPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &prev); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prev.HTML, "<script>alert") || strings.Contains(prev.HTML, "<b>x</b>") {
		t.Error("preview html contains unescaped markup")
	}
}

func TestEmailBranding_AppliedToSystemMail(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/email/branding", validBrandingBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d", rec.Code)
	}
	m, ok := rt.mailMessage(t.Context(), "password_reset", "sam@example.com", map[string]string{"reset_link": "https://example.com/r", "expires_in": "1 hour"}, mailtemplates.Options{})
	if !ok {
		t.Fatal("no message rendered")
	}
	if !strings.Contains(m.HTML, "cdn.example.com/logo.png") || !strings.Contains(m.Text, "Privacy: https://example.com/privacy") {
		t.Errorf("stored branding missing from a rich mail kind:\n%s", m.Text)
	}
}
