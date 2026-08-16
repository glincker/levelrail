package store

import (
	"context"
	"fmt"
)

// Email backends store.EmailSettings.Backend accepts. Empty string means
// "unset": the caller falls back to APP_SMTP_* env vars if set.
const (
	EmailBackendSMTP = "smtp"
	EmailBackendSES  = "ses"
)

// EmailSettingsSecretsKey is the internal/secrets serviceName the
// platform-wide email settings' credentials are stored under (envKeys
// "smtp_password" and "ses_secret_access_key").
func EmailSettingsSecretsKey() string {
	return "email-settings"
}

// EmailSettings is the single platform-wide email-sending configuration
// row. No credential fields: those go through internal/secrets instead.
type EmailSettings struct {
	// Backend is "", EmailBackendSMTP, or EmailBackendSES.
	Backend        string
	SMTPHost       string
	SMTPPort       int
	SMTPUsername   string
	SMTPFrom       string
	SESRegion      string
	SESAccessKeyID string
	SESFrom        string
}

// GetEmailSettings returns the single email_settings row. Always
// succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetEmailSettings(ctx context.Context) (EmailSettings, error) {
	var s EmailSettings
	err := db.QueryRowContext(ctx, `
		SELECT backend, smtp_host, smtp_port, smtp_username, smtp_from,
		       ses_region, ses_access_key_id, ses_from
		FROM email_settings
		WHERE id = 1
	`).Scan(&s.Backend, &s.SMTPHost, &s.SMTPPort, &s.SMTPUsername, &s.SMTPFrom,
		&s.SESRegion, &s.SESAccessKeyID, &s.SESFrom)
	if err != nil {
		return EmailSettings{}, fmt.Errorf("store: get email settings: %w", err)
	}
	return s, nil
}

// UpdateEmailSettings replaces the single email_settings row in full,
// the same whole-record convention UpdateIngressSettings uses.
func (db *DB) UpdateEmailSettings(ctx context.Context, s EmailSettings) error {
	_, err := db.ExecContext(ctx, `
		UPDATE email_settings
		SET backend = ?, smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_from = ?,
		    ses_region = ?, ses_access_key_id = ?, ses_from = ?
		WHERE id = 1
	`, s.Backend, s.SMTPHost, s.SMTPPort, s.SMTPUsername, s.SMTPFrom,
		s.SESRegion, s.SESAccessKeyID, s.SESFrom)
	if err != nil {
		return fmt.Errorf("store: update email settings: %w", err)
	}
	return nil
}
