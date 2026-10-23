package store

import (
	"context"
	"testing"
)

func TestEmailBranding_DefaultsAndRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetEmailBranding(ctx)
	if err != nil {
		t.Fatalf("GetEmailBranding() error = %v", err)
	}
	if got.LogoURL != "" || len(got.FooterLinks) != 0 || got.ShowPoweredBy {
		t.Fatalf("default branding = %+v, want empty with powered-by off", got)
	}

	want := EmailBranding{
		LogoURL: "https://cdn.example.com/logo.png", BrandColor: "#112233",
		FooterText: "Sent by {{brand_name}}", FooterAddress: "1 Main St",
		FooterLinks:   []EmailFooterLink{{Label: "Privacy", URL: "https://example.com/privacy"}},
		ShowPoweredBy: true,
	}
	if err := db.SetEmailBranding(ctx, want); err != nil {
		t.Fatalf("SetEmailBranding() error = %v", err)
	}
	got, err = db.GetEmailBranding(ctx)
	if err != nil {
		t.Fatalf("GetEmailBranding() error = %v", err)
	}
	if got.LogoURL != want.LogoURL || got.BrandColor != want.BrandColor || got.FooterText != want.FooterText ||
		got.FooterAddress != want.FooterAddress || len(got.FooterLinks) != 1 || got.FooterLinks[0] != want.FooterLinks[0] || !got.ShowPoweredBy {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}

	if err := db.UpdateEmailSettings(ctx, EmailSettings{Backend: EmailBackendResend, ResendFrom: "a@example.com"}); err != nil {
		t.Fatalf("UpdateEmailSettings() error = %v", err)
	}
	got, _ = db.GetEmailBranding(ctx)
	if got.LogoURL != want.LogoURL {
		t.Errorf("UpdateEmailSettings cleared branding: %+v", got)
	}
}
