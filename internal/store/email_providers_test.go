package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMigration2860_BackfillsConfiguredBackend(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	old, err := Open(ctx, path, WithMigrationCeiling(2850))
	if err != nil {
		t.Fatalf("open at 2850: %v", err)
	}
	if _, err := old.ExecContext(ctx, `UPDATE email_settings SET backend = 'mailgun', smtp_host = 'h', smtp_port = 465,
		smtp_username = 'u', smtp_from = 'sf', ses_region = 'eu-west-1', ses_access_key_id = 'AK', ses_from = 'ef',
		resend_from = 'rf', tls_mode = 'tls', tls_skip_verify = 1, ses_smtp_port = 2587, ses_smtp_username = 'su',
		mailgun_domain = 'mg.example.com', mailgun_region = 'eu', from_address = 'shared@example.com' WHERE id = 1`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	cur, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer func() { _ = cur.Close() }()

	got, err := cur.GetEmailProvider(ctx, EmailPrimaryProviderID)
	if err != nil {
		t.Fatalf("GetEmailProvider: %v", err)
	}
	want := EmailProvider{
		ID: EmailPrimaryProviderID, Role: EmailRolePrimary, Name: "mailgun", Enabled: true, Backend: EmailBackendMailgun,
		SMTPHost: "h", SMTPPort: 465, SMTPUsername: "u", SMTPFrom: "sf",
		SESRegion: "eu-west-1", SESAccessKeyID: "AK", SESFrom: "ef", ResendFrom: "rf",
		TLSMode: "tls", TLSSkipVerify: true, SESSMTPPort: 2587, SESSMTPUsername: "su",
		MailgunDomain: "mg.example.com", MailgunRegion: "eu",
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set: %+v", got)
	}
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	s, err := cur.GetEmailSettings(ctx)
	if err != nil || s.Backend != EmailBackendMailgun || s.FromAddress != "shared@example.com" {
		t.Errorf("email_settings changed: %+v err = %v", s, err)
	}
}

func TestMigration2860_EmptyBackendYieldsNoProvider(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListEmailProviders(context.Background())
	if err != nil {
		t.Fatalf("ListEmailProviders: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d providers, want 0", len(got))
	}
}

func TestEmailProvider_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	want := EmailProvider{
		ID: "prov_primary", Role: EmailRolePrimary, Name: "Main", Enabled: true, Backend: EmailBackendSMTP,
		SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPUsername: "apikey", SMTPFrom: "a@example.com",
		TLSMode: "starttls", TLSSkipVerify: true, SESSMTPPort: 587, MailgunRegion: "us",
		LastErrorKind: "auth",
	}
	if err := db.UpsertEmailProvider(ctx, want); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := db.GetEmailProvider(ctx, want.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set")
	}
	got.CreatedAt, got.UpdatedAt = want.CreatedAt, want.UpdatedAt
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}

	want.Enabled = false
	want.Name = "Renamed"
	if err := db.UpsertEmailProvider(ctx, want); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	list, err := db.ListEmailProviders(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Renamed" || list[0].Enabled {
		t.Errorf("list = %+v err = %v, want one updated row", list, err)
	}
}

func TestEmailProvider_Invariants(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	primary := EmailProvider{ID: "p1", Role: EmailRolePrimary, Enabled: true, Backend: EmailBackendResend, SESSMTPPort: 587, MailgunRegion: "us"}
	backup := EmailProvider{ID: "b1", Role: EmailRoleBackup, Enabled: true, Backend: EmailBackendPostmark, SESSMTPPort: 587, MailgunRegion: "us"}

	if err := db.UpsertEmailProvider(ctx, backup); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("backup without primary: err = %v, want ErrEmailProviderInvalid", err)
	}
	if err := db.UpsertEmailProvider(ctx, primary); err != nil {
		t.Fatalf("primary: %v", err)
	}
	if err := db.UpsertEmailProvider(ctx, backup); err != nil {
		t.Fatalf("backup: %v", err)
	}

	second := primary
	second.ID = "p2"
	if err := db.UpsertEmailProvider(ctx, second); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("second primary: err = %v, want ErrEmailProviderInvalid", err)
	}
	third := backup
	third.ID = "b2"
	if err := db.UpsertEmailProvider(ctx, third); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("second backup: err = %v, want ErrEmailProviderInvalid", err)
	}
	moved := backup
	moved.Role = EmailRolePrimary
	if err := db.UpsertEmailProvider(ctx, moved); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("role change: err = %v, want ErrEmailProviderInvalid", err)
	}
	bad := primary
	bad.Backend = "nope"
	if err := db.UpsertEmailProvider(ctx, bad); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("bad backend: err = %v, want ErrEmailProviderInvalid", err)
	}
	bad = primary
	bad.Role = "other"
	if err := db.UpsertEmailProvider(ctx, bad); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("bad role: err = %v, want ErrEmailProviderInvalid", err)
	}

	if err := db.DeleteEmailProvider(ctx, "p1"); !errors.Is(err, ErrEmailProviderInvalid) {
		t.Errorf("delete primary with backup: err = %v, want ErrEmailProviderInvalid", err)
	}
	if err := db.DeleteEmailProvider(ctx, "b1"); err != nil {
		t.Fatalf("delete backup: %v", err)
	}
	if err := db.DeleteEmailProvider(ctx, "p1"); err != nil {
		t.Fatalf("delete primary: %v", err)
	}
	if err := db.DeleteEmailProvider(ctx, "p1"); !errors.Is(err, ErrEmailProviderNotFound) {
		t.Errorf("delete missing: err = %v, want ErrEmailProviderNotFound", err)
	}
	if _, err := db.GetEmailProvider(ctx, "p1"); !errors.Is(err, ErrEmailProviderNotFound) {
		t.Errorf("get missing: err = %v, want ErrEmailProviderNotFound", err)
	}
}

func TestEmailProvider_ListOrdersPrimaryFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, p := range []EmailProvider{
		{ID: "p1", Role: EmailRolePrimary, Backend: EmailBackendSES},
		{ID: "b1", Role: EmailRoleBackup, Backend: EmailBackendSendGrid},
	} {
		if err := db.UpsertEmailProvider(ctx, p); err != nil {
			t.Fatalf("upsert %s: %v", p.ID, err)
		}
	}
	list, err := db.ListEmailProviders(ctx)
	if err != nil || len(list) != 2 || list[0].Role != EmailRolePrimary || list[1].Role != EmailRoleBackup {
		t.Errorf("list = %+v err = %v", list, err)
	}
}
