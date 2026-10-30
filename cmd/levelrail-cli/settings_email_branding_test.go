package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SettingsEmailBranding_SetKeepsUntouchedFieldsAndPreviewPrintsHTML(t *testing.T) {
	saved := apiclient.EmailBranding{LogoURL: "https://cdn.example.com/a.png", FooterText: "keep me"}
	var putBody apiclient.EmailBranding
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(saved)
		case r.Method == http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_ = json.NewEncoder(w).Encode(putBody)
		case r.URL.Path == "/api/v1/settings/email/branding/preview":
			_ = json.NewEncoder(w).Encode(apiclient.EmailBrandingPreview{HTML: "<p>preview</p>"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{
		"settings", "email", "branding", "set", "--brand-color", "#112233",
		"--footer-link", "Privacy|https://example.com/privacy", "--powered-by", "--api-url", srv.URL,
	})
	if putBody.LogoURL != saved.LogoURL || putBody.FooterText != "keep me" {
		t.Errorf("untouched fields lost: %+v", putBody)
	}
	if putBody.BrandColor != "#112233" || !putBody.ShowPoweredBy || len(putBody.FooterLinks) != 1 || putBody.FooterLinks[0].URL != "https://example.com/privacy" {
		t.Errorf("flags not applied: %+v", putBody)
	}

	stdout, _ := runCLIExpectOK(t, []string{"settings", "email", "branding", "preview", "--api-url", srv.URL})
	if !strings.Contains(stdout, "<p>preview</p>") {
		t.Errorf("preview stdout = %q", stdout)
	}
}
