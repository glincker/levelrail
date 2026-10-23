package store

import (
	"context"
	"errors"
	"testing"
)

func TestPromoteEmailProvider_SwapsRolesAndMirrorsSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	primary := EmailProvider{ID: "p1", Role: EmailRolePrimary, Name: "one", Enabled: true, Backend: EmailBackendResend, SESSMTPPort: 587, MailgunRegion: "us"}
	backup := EmailProvider{ID: "b1", Role: EmailRoleBackup, Name: "two", Enabled: true, Backend: EmailBackendSMTP, SMTPHost: "smtp.example.com", SMTPPort: 2525, SESSMTPPort: 587, MailgunRegion: "us"}
	for _, p := range []EmailProvider{primary, backup} {
		if err := db.UpsertEmailProvider(ctx, p); err != nil {
			t.Fatalf("seed %s: %v", p.ID, err)
		}
	}
	if err := db.PromoteEmailProvider(ctx, "b1"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	list, err := db.ListEmailProviders(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "b1" || list[0].Role != EmailRolePrimary || list[1].ID != "p1" || list[1].Role != EmailRoleBackup {
		t.Fatalf("after promote list = %+v err = %v", list, err)
	}
	if list[1].Name != "one" || list[1].CreatedAt.IsZero() {
		t.Errorf("old primary lost fields: %+v", list[1])
	}
	s, err := db.GetEmailSettings(ctx)
	if err != nil || s.Backend != EmailBackendSMTP || s.SMTPHost != "smtp.example.com" || s.SMTPPort != 2525 {
		t.Errorf("email_settings not mirrored: %+v err = %v", s, err)
	}
}

func TestPromoteEmailProvider_Errors(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.PromoteEmailProvider(ctx, "nope"); !errors.Is(err, ErrEmailProviderNotFound) {
		t.Errorf("missing: err = %v", err)
	}
	if err := db.UpsertEmailProvider(ctx, EmailProvider{ID: "p1", Role: EmailRolePrimary, Backend: EmailBackendSES}); err != nil {
		t.Fatal(err)
	}
	if err := db.PromoteEmailProvider(ctx, "p1"); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("already primary: err = %v", err)
	}
}

func TestEmailProvider_PrimaryWritesSyncSettings(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.UpsertEmailProvider(ctx, EmailProvider{ID: "p1", Role: EmailRolePrimary, Backend: EmailBackendResend, ResendFrom: "r@example.com"}); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.GetEmailSettings(ctx); s.Backend != EmailBackendResend || s.ResendFrom != "r@example.com" {
		t.Errorf("settings after upsert = %+v", s)
	}
	if err := db.DeleteEmailProvider(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if s, _ := db.GetEmailSettings(ctx); s.Backend != "" {
		t.Errorf("settings backend after delete = %q, want empty", s.Backend)
	}
}

func TestEmailProviderSecretsKey(t *testing.T) {
	if got := EmailProviderSecretsKey(EmailRolePrimary, "p1"); got != EmailSettingsSecretsKey() {
		t.Errorf("primary = %q", got)
	}
	if got := EmailProviderSecretsKey(EmailRoleBackup, "b1"); got != "email-settings/b1" {
		t.Errorf("backup = %q", got)
	}
}
