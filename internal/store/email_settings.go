package store

import (
	"context"
	"fmt"
)

// Email backends store.EmailSettings.Backend accepts (migrations/0031's
// own CHECK constraint). Empty string means "unset": internal/email's
// caller falls back to APP_SMTP_* env vars if those are set, or has no
// email capability at all, matching the migration's own upgrade-safe
// default.
const (
	EmailBackendSMTP = "smtp"
	EmailBackendSES  = "ses"
)

// EmailSettingsSecretsKey is the internal/secrets serviceName the
// platform-wide email settings' credentials are stored under (envKeys
// "smtp_password" and "ses_secret_access_key"). A function, not a
// constant format string inlined at each call site, the same reasoning
// BackupTargetSecretsKey's own doc comment gives: a fixed sentinel
// rather than a per-row ID, since there is exactly one email settings
// row, ever.
func EmailSettingsSecretsKey() string {
	return "email-settings"
}

// EmailSettings is the single platform-wide email-sending configuration
// row (migrations/0031_email_settings.sql). No credential fields here:
// see that migration's own doc comment for why the SMTP password and SES
// secret access key go through internal/secrets instead.
type EmailSettings struct {
	// Backend is "", EmailBackendSMTP, or EmailBackendSES. "" means no
	// backend is configured through this settings row (a caller may
	// still fall back to env vars, see internal/email's own callers).
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
// succeeds against a migrated database: the migration itself inserts the
// row (id = 1), the same "no not-found case" shape GetIngressSettings
// already establishes for its own singleton row.
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
// the same "whole record is always written as a whole" convention
// UpdateIngressSettings already establishes for its own singleton row.
// internal/api's PUT /api/v1/settings/email is expected to have already
// validated s; this method performs no validation of its own beyond what
// the schema itself enforces (the backend CHECK constraint).
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
