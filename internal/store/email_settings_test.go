package store

import (
	"context"
	"testing"
)

func TestGetEmailSettings_SeededDefault(t *testing.T) {
	db := openTestDB(t)

	got, err := db.GetEmailSettings(context.Background())
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if got.Backend != "" {
		t.Errorf("Backend = %q, want empty (unset by default)", got.Backend)
	}
	if got.SMTPPort != 0 || got.SMTPHost != "" || got.SESRegion != "" {
		t.Errorf("got %+v, want every field zero-valued on a fresh migration", got)
	}
}

func TestUpdateEmailSettings_RoundTrips(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := EmailSettings{
		Backend:        EmailBackendSMTP,
		SMTPHost:       "smtp.example.com",
		SMTPPort:       587,
		SMTPUsername:   "apikey",
		SMTPFrom:       "no-reply@example.com",
		SESRegion:      "",
		SESAccessKeyID: "",
		SESFrom:        "",
	}
	if err := db.UpdateEmailSettings(ctx, want); err != nil {
		t.Fatalf("UpdateEmailSettings() error = %v", err)
	}

	got, err := db.GetEmailSettings(ctx)
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestUpdateEmailSettings_SwitchingBackendReplacesInFull(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.UpdateEmailSettings(ctx, EmailSettings{
		Backend: EmailBackendSMTP, SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPFrom: "a@example.com",
	}); err != nil {
		t.Fatalf("first update: %v", err)
	}

	ses := EmailSettings{Backend: EmailBackendSES, SESRegion: "us-east-1", SESAccessKeyID: "AKID", SESFrom: "b@example.com"}
	if err := db.UpdateEmailSettings(ctx, ses); err != nil {
		t.Fatalf("second update: %v", err)
	}

	got, err := db.GetEmailSettings(ctx)
	if err != nil {
		t.Fatalf("GetEmailSettings() error = %v", err)
	}
	if got != ses {
		t.Errorf("got %+v, want %+v (a full replace, not a merge of the two updates)", got, ses)
	}
}

func TestEmailSettingsSecretsKey_Stable(t *testing.T) {
	if got := EmailSettingsSecretsKey(); got != EmailSettingsSecretsKey() {
		t.Errorf("EmailSettingsSecretsKey() is not stable across calls: %q vs %q", got, EmailSettingsSecretsKey())
	}
}
